package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"vera-app/backend/currency"
)

// CompanyProfile is the singleton company record.
//
// Only the name and address print on a quotation -- phone, email and NPWP appear
// nowhere in the template, so they are recorded for invoices and admin rather
// than for customers. The eight theme colours are split into a quotation set and
// a portal set, independently settable; an empty one means the stylesheet's own
// default.
type CompanyProfile struct {
	LegalName string `json:"LegalName"`
	Address   string `json:"Address"`
	Phone     string `json:"Phone,omitempty"`
	Email     string `json:"Email,omitempty"`
	TaxID     string `json:"TaxID,omitempty"`
	// Currency is the company's ISO 4217 code from the shipped catalogue.
	// Empty reads as IDR.
	Currency string `json:"Currency,omitempty"`
	HasLogo  bool   `json:"HasLogo"`

	ThemeQDeep   string `json:"ThemeQDeep,omitempty"`
	ThemeQAccent string `json:"ThemeQAccent,omitempty"`
	ThemeQSoft   string `json:"ThemeQSoft,omitempty"`
	ThemeQInk    string `json:"ThemeQInk,omitempty"`

	ThemePDeep   string `json:"ThemePDeep,omitempty"`
	ThemePAccent string `json:"ThemePAccent,omitempty"`
	ThemePSoft   string `json:"ThemePSoft,omitempty"`
	ThemePInk    string `json:"ThemePInk,omitempty"`
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func companyCommands() *command {
	return commandGroup("company", "The company record that heads every document.",
		companyShowCommand(),
		companySetCommand(),
		companyThemeCommand(),
		companyLogoCommand(),
	)
}

func companyShowCommand() *command {
	return &command{
		name:    "show",
		summary: "Show the company record and brand colours.",
		usage:   "vera company show",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var profile CompanyProfile
			if err := client.Get("/v1/admin/company", &profile); err != nil {
				return err
			}
			if err := a.emitJSON(profile); err != nil {
				return err
			}
			a.field("Legal name", profile.LegalName)
			a.field("Address", profile.Address)
			a.field("Phone", profile.Phone)
			a.field("Email", profile.Email)
			a.field("NPWP", profile.TaxID)
			a.field("Currency", currency.Normalize(profile.Currency))
			a.field("Logo", fmt.Sprintf("%v", profile.HasLogo))
			a.field("Quotation theme", colourSummary(profile.ThemeQDeep, profile.ThemeQAccent, profile.ThemeQSoft, profile.ThemeQInk))
			a.field("Portal theme", colourSummary(profile.ThemePDeep, profile.ThemePAccent, profile.ThemePSoft, profile.ThemePInk))
			a.note("Only the name and address print on a quotation.")
			return nil
		},
	}
}

func colourSummary(deep, accent, soft, ink string) string {
	parts := []string{}
	for _, c := range []struct{ label, value string }{
		{"deep", deep}, {"accent", accent}, {"soft", soft}, {"ink", ink},
	} {
		if c.value != "" {
			parts = append(parts, c.label+" "+c.value)
		}
	}
	if len(parts) == 0 {
		return "(stylesheet default)"
	}
	return strings.Join(parts, "  ")
}

func companySetCommand() *command {
	var name, address, phone, email, taxID, currencyCode string
	return &command{
		name:    "set",
		summary: "Change the company record (admin).",
		usage:   `vera company set [--name "..."] [--address "..."] [--phone "..."] [--email "..."] [--npwp "..."] [--currency USD]`,
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "legal name, printed on every document")
			fs.StringVar(&address, "address", "", "address, printed under the name")
			fs.StringVar(&phone, "phone", "", "phone (recorded, never printed on a quotation)")
			fs.StringVar(&email, "email", "", "email (recorded, never printed on a quotation)")
			fs.StringVar(&taxID, "npwp", "", "tax id (dormant, for invoices)")
			fs.StringVar(&currencyCode, "currency", "", "ISO 4217 code from the catalogue ("+strings.Join(currency.Codes(), ", ")+")")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if currencyCode != "" {
				if _, ok := currency.Lookup(currencyCode); !ok {
					return usagef(a.cmd, "--currency must be one of %s, got %q",
						strings.Join(currency.Codes(), ", "), currencyCode)
				}
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			// The endpoint replaces the record, so the current one is the base
			// and only the named fields move. Sending a partial body would blank
			// the fields nobody mentioned.
			var profile CompanyProfile
			if err := client.Get("/v1/admin/company", &profile); err != nil {
				return err
			}
			if name != "" {
				profile.LegalName = name
			}
			if address != "" {
				profile.Address = address
			}
			if phone != "" {
				profile.Phone = phone
			}
			if email != "" {
				profile.Email = email
			}
			if taxID != "" {
				profile.TaxID = taxID
			}
			if currencyCode != "" {
				profile.Currency = currency.Normalize(currencyCode)
			}
			if err := client.Put("/v1/admin/company", profile, &profile); err != nil {
				return err
			}
			if err := a.emitJSON(profile); err != nil {
				return err
			}
			a.line("Updated.")
			a.note("Header edits affect future quotations; archived documents keep theirs.")
			return nil
		},
	}
}

// companyThemeCommand sets the brand colours. It is separate from `set` because
// the two are changed for entirely different reasons, and a mistyped colour
// should not be able to ride along with a corrected address.
func companyThemeCommand() *command {
	var deep, accent, soft, ink string
	var portal bool
	return commandGroup("theme", "The brand colours.",
		&command{
			name:    "set",
			summary: "Set the quotation (or portal) brand colours (admin).",
			usage:   `vera company theme set --deep "#244f39" [--accent ...] [--soft ...] [--ink ...] [--portal]`,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&deep, "deep", "", "the darkest colour, used for headings")
				fs.StringVar(&accent, "accent", "", "the accent, used for totals and emphasis")
				fs.StringVar(&soft, "soft", "", "the soft background")
				fs.StringVar(&ink, "ink", "", "the body text colour")
				fs.BoolVar(&portal, "portal", false, "set the verification portal's colours instead of the quotation's")
			},
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				for label, value := range map[string]string{
					"--deep": deep, "--accent": accent, "--soft": soft, "--ink": ink,
				} {
					if value != "" && !hexColour.MatchString(value) {
						return usagef(a.cmd, "%s must be a #RRGGBB colour, got %q", label, value)
					}
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				var profile CompanyProfile
				if err := client.Get("/v1/admin/company", &profile); err != nil {
					return err
				}
				if portal {
					setIfGiven(&profile.ThemePDeep, deep)
					setIfGiven(&profile.ThemePAccent, accent)
					setIfGiven(&profile.ThemePSoft, soft)
					setIfGiven(&profile.ThemePInk, ink)
				} else {
					setIfGiven(&profile.ThemeQDeep, deep)
					setIfGiven(&profile.ThemeQAccent, accent)
					setIfGiven(&profile.ThemeQSoft, soft)
					setIfGiven(&profile.ThemeQInk, ink)
				}
				if err := client.Put("/v1/admin/company", profile, &profile); err != nil {
					return err
				}
				if err := a.emitJSON(profile); err != nil {
					return err
				}
				a.field("Quotation theme", colourSummary(profile.ThemeQDeep, profile.ThemeQAccent, profile.ThemeQSoft, profile.ThemeQInk))
				a.field("Portal theme", colourSummary(profile.ThemePDeep, profile.ThemePAccent, profile.ThemePSoft, profile.ThemePInk))
				return nil
			},
		},
	)
}

func setIfGiven(target *string, value string) {
	if value != "" {
		*target = value
	}
}

// allowedLogoTypes mirrors the server's allowlist. Checking here turns "the
// server refused a 3MB .webp" into a sentence naming the file.
var allowedLogoTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
}

func companyLogoCommand() *command {
	var out string
	return commandGroup("logo", "The company artwork.",
		&command{
			name:    "set",
			summary: "Upload a logo (admin). It prints profile-first, with the built-in one as fallback.",
			usage:   "vera company logo set <file.png>",
			run: func(a *app) error {
				if err := a.requireArgs(1, "a logo file"); err != nil {
					return err
				}
				path := a.args[0]
				mime, ok := allowedLogoTypes[strings.ToLower(filepath.Ext(path))]
				if !ok {
					return usagef(a.cmd, "%s is not an allowed image type (png, jpg, svg, webp)", filepath.Ext(path))
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read %s: %w", path, err)
				}
				// 512KB is the server's cap; checking here keeps a rejected
				// upload from being sent at all.
				if len(data) > 512*1024 {
					return fmt.Errorf("%s is %dKB, over the 512KB limit", path, len(data)/1024)
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				var response struct {
					HasLogo bool `json:"hasLogo"`
				}
				if err := client.Put("/v1/admin/company/logo", map[string]string{
					"logoBase64": base64.StdEncoding.EncodeToString(data),
					"logoMime":   mime,
				}, &response); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"uploaded": true, "bytes": len(data), "mime": mime}); err != nil {
					return err
				}
				a.line("Uploaded %s (%dKB).", path, len(data)/1024)
				return nil
			},
		},
		&command{
			name:    "get",
			summary: "Download the uploaded logo.",
			usage:   "vera company logo get [--out logo.png]",
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&out, "out", "", "where to write it (default: the name the server reports)")
			},
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				// The server answers with a data URL inside JSON.
				var response struct {
					LogoBase64 string `json:"logoBase64"`
					LogoMime   string `json:"logoMime"`
				}
				if err := client.Get("/v1/admin/company/logo", &response); err != nil {
					return err
				}
				if response.LogoBase64 == "" {
					return fmt.Errorf("no logo has been uploaded; the built-in artwork is used")
				}
				data, err := base64.StdEncoding.DecodeString(response.LogoBase64)
				if err != nil {
					return fmt.Errorf("decode the logo: %w", err)
				}
				destination := out
				if destination == "" {
					destination = "company-logo" + extensionForMime(response.LogoMime)
				}
				if err := writeFile(destination, data); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"out": destination, "bytes": len(data), "mime": response.LogoMime}); err != nil {
					return err
				}
				a.line("Wrote %s (%d bytes).", destination, len(data))
				return nil
			},
		},
		&command{
			name:    "clear",
			summary: "Remove the uploaded logo, falling back to the built-in artwork (admin).",
			usage:   "vera company logo clear",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				if err := a.confirm("Remove the uploaded logo? Documents will use the built-in artwork."); err != nil {
					return err
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				if err := client.Delete("/v1/admin/company/logo", nil); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"cleared": true}); err != nil {
					return err
				}
				a.line("Cleared.")
				return nil
			},
		},
	)
}

func extensionForMime(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	}
	return ".png"
}
