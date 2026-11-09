/**
 * Renders a release version with exactly one leading `v`.
 *
 * The release contract already serves `v`-prefixed versions (`v0.15.0`), and
 * the Go build stamps `AppVersion` the same way. Older API responses and test
 * fixtures sometimes omit the prefix, and a UI that blindly prepends one
 * produced `vv0.15.0` on the login page (#546). Normalising here means every
 * surface renders the same label whatever shape it was handed.
 *
 * `dev` is the unreleased-build sentinel and stays unprefixed.
 */
export function displayVersion(value: string | null | undefined): string {
  const trimmed = (value ?? "").trim();
  if (trimmed === "") return "dev";
  const withoutPrefix = trimmed.replace(/^v+/, "");
  if (withoutPrefix === "" || withoutPrefix.toLowerCase() === "dev") return "dev";
  return `v${withoutPrefix}`;
}
