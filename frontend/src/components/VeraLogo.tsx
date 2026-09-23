// VeraLogo is the official Vera brand mark, inlined so it renders identically
// in the desktop WebView and plain-browser dev with no asset pipeline.
export function VeraLogo({ className = "h-6 w-auto" }: { className?: string }) {
  return (
    <svg viewBox="0 0 409 300" fill="none" xmlns="http://www.w3.org/2000/svg" className={className} aria-hidden="true">
      <path d="M0.865967 1L95.1316 1L194.366 194L179.366 222L78.366 27H46.6316L172.783 245.5L300.377 24.5H331.132L172.783 298.768L0.865967 1Z" fill="#2B2A27" stroke="#2B2A27" />
      <path d="M300.366 24.5L314.222 0.5H407.866L235.866 298.413L207.366 250L223.366 222.946L235.866 244.751L363.028 24.5H300.366Z" fill="#E6D0A1" stroke="#2B2A27" />
      <path d="M154.866 0.760742H199.366C199.366 0.760742 199.366 22.2607 199.366 43.7607C199.366 65.2607 171.866 81.7607 171.866 81.7607L161.366 68.8548C163.483 70.3431 187.818 43.7607 176.366 43.7607C175.192 43.7607 154.866 43.7607 154.866 43.7607V0.760742Z" fill="#E6D0A1" stroke="#2B2A27" />
      <path d="M214.366 0.5H258.866C258.866 0.5 258.866 22 258.866 43.5C258.866 65 231.366 81.5 231.366 81.5L220.866 68.594C222.983 70.0823 247.318 43.5 235.866 43.5C234.692 43.5 214.366 43.5 214.366 43.5V0.5Z" fill="#E6D0A1" stroke="#2B2A27" />
    </svg>
  );
}
