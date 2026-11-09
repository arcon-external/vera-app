/** The message a Wails rejection carries, in every shape the runtime uses.
 *
 *  Wails hands back a bare string for some failures, an Error for others, a
 *  `{message}` object for the typed Go errors, and a `{code}` refusal when the
 *  server sent a stable code but no prose. Reading only the string was issue
 *  #531's root cause: every other shape silently became a generic sentence,
 *  and the session poll treated the missing message as a sign-out.
 */
export function errorMessage(error: unknown, fallback: string): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
    const code = (error as { code?: unknown }).code;
    // Stable server codes are snake_case. A bare word is not one, so it keeps
    // the caller's fallback rather than surfacing an internal token.
    if (
      typeof code === "string" &&
      /^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$/i.test(code.trim())
    ) {
      return readableCode(code);
    }
  }
  return fallback;
}

/** A stable code is not prose, but it is better than a generic sentence when
 *  it is the only thing the refusal carried: `session_check_failed` becomes
 *  "Session check failed". */
function readableCode(code: string): string {
  const text = code.trim().replace(/[_-]+/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}
