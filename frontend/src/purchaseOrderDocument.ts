// Issue #472: the pure decisions behind the single customer PO PDF.
//
// The panel renders these, and buildFinalizeInput reads the same confirmation
// rules, so a checkbox on screen and the payload that is sent cannot disagree.
// Nothing here parses a PDF: the server is the validation authority. The
// client only bounds the file size before uploading, because a 20 MB rejected
// round trip is wasteful.

/** The one active PDF as the desktop binding returns it (PascalCase = Wails). */
export interface PurchaseOrderDocumentDTO {
  UUID: string;
  SHA256: string;
  ContentType: string;
  OriginalFilename: string;
  UploadedAt: string;
  ReplacedAt?: string | null;
  RemovedAt?: string | null;
  RemovedBy?: number | null;
  ID: number;
  PurchaseOrderID: number;
  Version: number;
  SizeBytes: number;
  UploadedBy: number;
  Active: boolean;
}

/** The active document plus retained history, newest first. */
export interface PurchaseOrderDocumentStateDTO {
  Document?: PurchaseOrderDocumentDTO | null;
  Versions: PurchaseOrderDocumentDTO[];
}

/**
 * The uploaded PDF and the headers identifying the version served.
 *
 * DataBase64 is the explicit wire representation: Wails serializes a Go []byte
 * as base64 text, so a number[] declaration (the generated binding's first
 * shape) described a value that never arrived. The panel decodes this string
 * with base64ToBytes, and those are the only bytes it previews.
 */
export interface PurchaseOrderDocumentContentDTO {
  DataBase64: string;
  ContentType: string;
  Filename: string;
  SHA256: string;
  Version: number;
}

/** The attach/replace payload (PascalCase = Wails binding). */
export interface PurchaseOrderDocumentUploadInput {
  PurchaseOrderID: number;
  DataBase64: string;
  Filename: string;
}

/** The review/finalize document branch. */
export type PurchaseOrderDocumentState = "none" | "attached";

/** The upload bound shared with the Go validator and the schema. */
export const MAX_PURCHASE_ORDER_DOCUMENT_BYTES = 20 * 1024 * 1024;

/** The exact empty-state sentence staff see, pinned by tests. */
export const NO_DOCUMENT_TEXT = "No customer PO PDF attached";

const BASE64_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
const BASE64_VALUES = new Int16Array(128).fill(-1);
for (let index = 0; index < BASE64_ALPHABET.length; index += 1) {
  BASE64_VALUES[BASE64_ALPHABET.charCodeAt(index)] = index;
}

/**
 * Encodes file bytes for the binding. Chunked so a 20 MB attachment does not
 * build its base64 text by one-character string concatenation.
 */
export function bytesToBase64(bytes: Uint8Array): string {
  const chunks: string[] = [];
  let chunk = "";
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index];
    const hasSecond = index + 1 < bytes.length;
    const hasThird = index + 2 < bytes.length;
    const second = hasSecond ? bytes[index + 1] : 0;
    const third = hasThird ? bytes[index + 2] : 0;
    chunk += BASE64_ALPHABET[first >> 2];
    chunk += BASE64_ALPHABET[((first & 0x03) << 4) | (second >> 4)];
    chunk += hasSecond ? BASE64_ALPHABET[((second & 0x0f) << 2) | (third >> 6)] : "=";
    chunk += hasThird ? BASE64_ALPHABET[third & 0x3f] : "=";
    if (chunk.length >= 8192) {
      chunks.push(chunk);
      chunk = "";
    }
  }
  if (chunk.length > 0 || chunks.length === 0) chunks.push(chunk);
  return chunks.join("");
}

/**
 * Decodes the binding's base64 text into the exact bytes. Padding is required
 * and must be at the end: silently accepting a short or malformed value would
 * preview a truncated PDF as if it were the stored one.
 */
export function base64ToBytes(value: string): Uint8Array<ArrayBuffer> {
  if (typeof value !== "string") {
    throw new Error("the customer PO PDF content is not base64 text");
  }
  const clean = value.replace(/[\r\n\t ]/g, "");
  if (clean.length === 0) return new Uint8Array(0);
  if (clean.length % 4 !== 0) {
    throw new Error("the customer PO PDF content is not valid base64");
  }
  const padding = clean.endsWith("==") ? 2 : clean.endsWith("=") ? 1 : 0;
  const dataLength = clean.length - padding;
  const bytes = new Uint8Array((clean.length / 4) * 3 - padding);
  let cursor = 0;
  for (let index = 0; index < clean.length; index += 4) {
    const values = [0, 0, 0, 0];
    for (let offset = 0; offset < 4; offset += 1) {
      const character = clean[index + offset];
      const position = index + offset;
      if (position >= dataLength) {
        if (character !== "=") {
          throw new Error("the customer PO PDF content is not valid base64");
        }
        continue;
      }
      if (character === "=") {
        throw new Error("the customer PO PDF content is not valid base64");
      }
      const code = character.charCodeAt(0);
      const decoded = code < 128 ? BASE64_VALUES[code] : -1;
      if (decoded < 0) {
        throw new Error("the customer PO PDF content is not valid base64");
      }
      values[offset] = decoded;
    }
    bytes[cursor] = (values[0] << 2) | (values[1] >> 4);
    cursor += 1;
    if (cursor < bytes.length) {
      bytes[cursor] = ((values[1] & 0x0f) << 4) | (values[2] >> 2);
      cursor += 1;
    }
    if (cursor < bytes.length) {
      bytes[cursor] = ((values[2] & 0x03) << 6) | values[3];
      cursor += 1;
    }
  }
  return bytes;
}

/** Reads the branch from the server state. */
export function documentStateOf(
  state: PurchaseOrderDocumentStateDTO | null | undefined
): PurchaseOrderDocumentState {
  return state?.Document != null ? "attached" : "none";
}

/** "Version 2 of 2", or the empty-state sentence. */
export function documentVersionLabel(
  state: PurchaseOrderDocumentStateDTO | null | undefined
): string {
  if (state?.Document == null) return NO_DOCUMENT_TEXT;
  const total = state.Versions.length > 0 ? state.Versions.length : 1;
  return `Version ${state.Document.Version} of ${total}`;
}

/** Removal always asks first when there is a current PDF to remove. */
export function removalRequiresConfirmation(
  state: PurchaseOrderDocumentStateDTO | null | undefined
): boolean {
  return documentStateOf(state) === "attached";
}

/** The final save's no-PDF confirmation applies only in the "none" branch. */
export function finalSaveNeedsConfirmation(
  documentState: PurchaseOrderDocumentState,
  confirmed: boolean
): boolean {
  return documentState === "none" && !confirmed;
}

/**
 * Whether the final-save payload records the absence. An attached save never
 * does, even if a client sent the checkbox; the server also nulls it.
 */
export function finalSaveSendsAbsenceConfirmation(
  documentState: PurchaseOrderDocumentState,
  confirmed: boolean
): boolean {
  return documentState === "none" && confirmed;
}

function messageOf(error: unknown): string {
  if (typeof error === "string") return error.trim();
  if (error instanceof Error) return error.message.trim();
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string") return message.trim();
  }
  return "";
}

/**
 * Turns a server refusal into the sentence staff read. Encryption is checked
 * before the generic invalid-PDF wording because both messages contain "PDF".
 */
export function documentErrorMessage(error: unknown): string {
  const message = messageOf(error);
  const lower = message.toLowerCase();
  if (lower.includes("password-protected") || lower.includes("unlocked") || lower.includes("encrypt")) {
    return "This PDF is password-protected. Attach an unlocked copy; Vera does not store PDF passwords.";
  }
  if (lower.includes("larger than 20 mb") || lower.includes("too large")) {
    return "This PDF is larger than 20 MB. Choose a smaller file.";
  }
  if (lower.includes("base64")) {
    return "The attached PDF could not be opened. Attach it again.";
  }
  if (lower.includes("not a valid pdf") || lower.includes("valid pdf")) {
    return "This file is not a valid PDF. Choose a PDF that opens in a PDF reader.";
  }
  if (lower.includes("confirm removing")) {
    return "Confirm removing the customer PO PDF.";
  }
  if (lower.includes("not found")) {
    return "The customer PO document is no longer available. Reload the purchase order.";
  }
  return message === "" ? "The customer PO PDF could not be updated. Try again." : message;
}

/**
 * Binary file size with one decimal place, computed with integers. No float
 * arithmetic and no locale-dependent formatting.
 */
export function formatDocumentSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const scale = bytes < 1024 * 1024 ? 1024 : 1024 * 1024;
  const unit = scale === 1024 ? "KB" : "MB";
  const tenths = Math.round((bytes * 10) / scale);
  const whole = Math.floor(tenths / 10);
  const fraction = tenths % 10;
  return fraction === 0 ? `${whole} ${unit}` : `${whole}.${fraction} ${unit}`;
}
