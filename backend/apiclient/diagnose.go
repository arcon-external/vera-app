package apiclient

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// DiagnoseOptions bounds one diagnosis run.
//
// The stage timeouts exist so a dead line fails at the stage that is dead
// instead of at the whole-request timeout. The CLI leaves Resolver and
// TLSConfig nil; the tests use them to make DNS and a self-signed local server
// deterministic.
type DiagnoseOptions struct {
	Timeout       time.Duration
	DialTimeout   time.Duration
	TLSTimeout    time.Duration
	HeaderTimeout time.Duration
	// Requests is how many probes to run. The first measures the stages; a
	// second (the default) checks whether the connection was reused. 1 skips
	// the reuse probe.
	Requests  int
	Resolver  *net.Resolver
	TLSConfig *tls.Config
}

// DiagnoseStage is one measured step of the connection.
type DiagnoseStage struct {
	Name       string  `json:"name"`
	DurationMS float64 `json:"durationMs"`
	Detail     string  `json:"detail,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// DiagnoseReport is the machine-readable result of one diagnosis.
//
// OK reports whether the request reached the server, not whether the server
// liked the answer: Status and HTTPVersion carry the response. A failed
// transport leaves OK false and names ErrorStage.
type DiagnoseReport struct {
	URL         string          `json:"url"`
	OK          bool            `json:"ok"`
	Status      int             `json:"status,omitempty"`
	HTTPVersion string          `json:"httpVersion,omitempty"`
	RemoteAddr  string          `json:"remoteAddr,omitempty"`
	Reused      bool            `json:"reused"`
	ReuseMS     float64         `json:"reuseMs,omitempty"`
	ReuseError  string          `json:"reuseError,omitempty"`
	DNSMS       float64         `json:"dnsMs"`
	ConnectMS   float64         `json:"connectMs"`
	TLSMS       float64         `json:"tlsMs"`
	TTFBMS      float64         `json:"ttfbMs"`
	TotalMS     float64         `json:"totalMs"`
	Stages      []DiagnoseStage `json:"stages"`
	ErrorStage  string          `json:"errorStage,omitempty"`
	Error       string          `json:"error,omitempty"`
}

const (
	diagnoseDefaultTimeout = 15 * time.Second
	diagnoseDialTimeout    = 5 * time.Second
	diagnoseTLSTimeout     = 5 * time.Second
	diagnoseHeaderTimeout  = 10 * time.Second
)

// Diagnose measures the API connection against <baseURL>/health.
//
// It is deliberately unauthenticated and does not use the session: the
// question is whether this machine can reach the server at all, and a token
// would add a failure mode without answering it.
func Diagnose(ctx context.Context, baseURL string, opts DiagnoseOptions) DiagnoseReport {
	base := strings.TrimRight(baseURL, "/")
	report := DiagnoseReport{URL: DiagnoseURL(base)}
	if base == "" {
		report.ErrorStage = "request"
		report.Error = "API URL is empty"
		return report
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = diagnoseDefaultTimeout
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = diagnoseDialTimeout
	}
	tlsTimeout := opts.TLSTimeout
	if tlsTimeout <= 0 {
		tlsTimeout = diagnoseTLSTimeout
	}
	headerTimeout := opts.HeaderTimeout
	if headerTimeout <= 0 {
		headerTimeout = diagnoseHeaderTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	if opts.Resolver != nil {
		dialer.Resolver = opts.Resolver
	}
	transport.DialContext = dialer.DialContext
	transport.TLSHandshakeTimeout = tlsTimeout
	transport.ResponseHeaderTimeout = headerTimeout
	transport.ForceAttemptHTTP2 = true
	transport.MaxIdleConnsPerHost = 4
	if opts.TLSConfig != nil {
		transport.TLSClientConfig = opts.TLSConfig
	}
	client := &http.Client{Transport: transport}

	first := probe(ctx, client, report.URL)
	report.OK = first.err == nil
	report.Status = first.status
	report.HTTPVersion = first.version
	report.RemoteAddr = first.remote
	report.DNSMS = milliseconds(first.dns)
	report.ConnectMS = connectMilliseconds(first.connects)
	report.TLSMS = milliseconds(first.tls)
	report.TTFBMS = milliseconds(first.ttfb)
	report.TotalMS = milliseconds(first.total)
	report.Stages = first.stages
	if first.err != nil {
		report.ErrorStage = first.errorStage
		report.Error = first.err.Error()
		return report
	}

	if opts.Requests != 1 {
		reuse := probe(ctx, client, report.URL)
		report.Reused = reuse.reused
		report.ReuseMS = milliseconds(reuse.total)
		if reuse.err != nil {
			report.ReuseError = reuse.err.Error()
		}
	}
	return report
}

// connectAttempt is one TCP connect the transport tried. Happy Eyeballs can
// produce more than one, so the report keeps them all.
type connectAttempt struct {
	network string
	addr    string
	start   time.Time
	dur     time.Duration
	err     error
}

type probeResult struct {
	dns        time.Duration
	dnsAddrs   []string
	dnsErr     error
	dnsStart   bool
	dnsDone    bool
	connects   []connectAttempt
	tls        time.Duration
	tlsErr     error
	tlsStart   bool
	tlsDone    bool
	ttfb       time.Duration
	total      time.Duration
	status     int
	version    string
	remote     string
	reused     bool
	wrote      bool
	gotConn    bool
	firstByte  bool
	err        error
	errorStage string
	stages     []DiagnoseStage
}

// probe runs one request with a trace and returns the measured stages.
func probe(ctx context.Context, client *http.Client, url string) probeResult {
	var r probeResult
	var dnsStart, tlsStart time.Time
	start := time.Now()

	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			r.dnsStart = true
			dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			r.dnsDone = true
			r.dns = time.Since(dnsStart)
			r.dnsErr = info.Err
			for _, addr := range info.Addrs {
				r.dnsAddrs = append(r.dnsAddrs, addr.String())
			}
		},
		ConnectStart: func(network, addr string) {
			r.connects = append(r.connects, connectAttempt{network: network, addr: addr, start: time.Now()})
		},
		ConnectDone: func(network, addr string, err error) {
			for i := len(r.connects) - 1; i >= 0; i-- {
				a := &r.connects[i]
				if a.network == network && a.addr == addr && a.dur == 0 {
					a.dur = time.Since(a.start)
					a.err = err
					return
				}
			}
		},
		TLSHandshakeStart: func() {
			r.tlsStart = true
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			r.tlsDone = true
			r.tls = time.Since(tlsStart)
			r.tlsErr = err
		},
		GotConn: func(info httptrace.GotConnInfo) {
			r.gotConn = true
			r.reused = info.Reused
			if info.Conn != nil {
				r.remote = info.Conn.RemoteAddr().String()
			}
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			r.wrote = true
		},
		GotFirstResponseByte: func() {
			r.firstByte = true
			r.ttfb = time.Since(start)
		},
	}

	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, url, nil)
	if err != nil {
		r.err = err
		r.errorStage = "request"
		r.total = time.Since(start)
		r.stages = stagesOf(&r)
		return r
	}
	request.Header.Set("User-Agent", "vera-net-diagnose")

	response, err := client.Do(request)
	r.total = time.Since(start)
	if err != nil {
		r.err = err
		r.errorStage = classify(&r)
		r.stages = stagesOf(&r)
		return r
	}
	r.status = response.StatusCode
	r.version = response.Proto
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	r.total = time.Since(start)
	r.stages = stagesOf(&r)
	return r
}

// classify names the stage that stopped the request. A stage that started but
// never finished is the stage that was waiting when the deadline hit.
func classify(r *probeResult) string {
	if r.dnsStart && !r.dnsDone {
		return "dns"
	}
	if r.dnsErr != nil {
		return "dns"
	}
	connected := false
	var connectErr error
	for _, a := range r.connects {
		if a.err == nil && a.dur > 0 {
			connected = true
		}
		if a.err != nil {
			connectErr = a.err
		}
	}
	if !connected && (connectErr != nil || len(r.connects) > 0) {
		return "connect"
	}
	if r.tlsStart && !r.tlsDone {
		return "tls"
	}
	if r.tlsErr != nil {
		return "tls"
	}
	if r.gotConn || r.wrote {
		return "header"
	}
	return "request"
}

func stagesOf(r *probeResult) []DiagnoseStage {
	out := make([]DiagnoseStage, 0, len(r.connects)+4)
	if r.dns > 0 || r.dnsErr != nil || r.dnsStart {
		out = append(out, DiagnoseStage{
			Name:       "dns",
			DurationMS: milliseconds(r.dns),
			Detail:     strings.Join(r.dnsAddrs, ", "),
			Error:      errorString(r.dnsErr),
		})
	}
	for _, a := range r.connects {
		out = append(out, DiagnoseStage{
			Name:       "connect",
			DurationMS: milliseconds(a.dur),
			Detail:     a.network + " " + a.addr,
			Error:      errorString(a.err),
		})
	}
	if r.tls > 0 || r.tlsErr != nil || r.tlsStart {
		out = append(out, DiagnoseStage{
			Name:       "tls",
			DurationMS: milliseconds(r.tls),
			Error:      errorString(r.tlsErr),
		})
	}
	if r.firstByte {
		out = append(out, DiagnoseStage{
			Name:       "ttfb",
			DurationMS: milliseconds(r.ttfb),
		})
	}
	out = append(out, DiagnoseStage{
		Name:       "total",
		DurationMS: milliseconds(r.total),
	})
	return out
}

func connectMilliseconds(connects []connectAttempt) float64 {
	for _, a := range connects {
		if a.err == nil && a.dur > 0 {
			return milliseconds(a.dur)
		}
	}
	if len(connects) > 0 {
		return milliseconds(connects[0].dur)
	}
	return 0
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// DiagnoseURL is the health endpoint a diagnosis uses.
func DiagnoseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/health"
}
