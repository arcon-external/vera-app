// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PurchaseOrderDocumentStateDTO } from "../purchaseOrderDocument";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";

// Issue #472: the attachment panel in a real DOM. The bindings are replaced at
// their module boundary; everything inside the panel is real. These cover the
// interactions SSR cannot: choosing a file, attach/replace, the separate
// removal confirmation, the inline validation message, and the PDF preview.

const { stateMock, attachMock, contentMock, removeMock } = vi.hoisted(() => ({
  stateMock: vi.fn(),
  attachMock: vi.fn(),
  contentMock: vi.fn(),
  removeMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderDocumentState: (...args: unknown[]) => stateMock(...args),
  AttachPurchaseOrderDocument: (...args: unknown[]) => attachMock(...args),
  GetActivePurchaseOrderDocumentContent: (...args: unknown[]) => contentMock(...args),
  RemovePurchaseOrderDocument: (...args: unknown[]) => removeMock(...args),
}));

vi.mock("@backend/models", () => ({
  backend: {
    PurchaseOrderDocumentUploadInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
  },
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function stateWith(version: number | null, versions = version === null ? [] : [version]): PurchaseOrderDocumentStateDTO {
  return {
    Document:
      version === null
        ? null
        : {
            UUID: `doc-${version}`,
            SHA256: `sha-${version}`,
            ContentType: "application/pdf",
            OriginalFilename: `Customer PO ${version}.pdf`,
            UploadedAt: "2026-10-21T10:00:00Z",
            ID: version,
            PurchaseOrderID: 7,
            Version: version,
            SizeBytes: 1536,
            UploadedBy: 4,
            Active: true,
          },
    Versions: versions.map((item) => ({
      UUID: `doc-${item}`,
      SHA256: `sha-${item}`,
      ContentType: "application/pdf",
      OriginalFilename: `Customer PO ${item}.pdf`,
      UploadedAt: "2026-10-21T10:00:00Z",
      ID: item,
      PurchaseOrderID: 7,
      Version: item,
      SizeBytes: 1536,
      UploadedBy: 4,
      Active: item === version,
    })),
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  stateMock.mockReset();
  attachMock.mockReset();
  contentMock.mockReset();
  removeMock.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string, scope: ParentNode = document): HTMLButtonElement {
  const found = Array.from(
    scope.querySelectorAll<HTMLButtonElement>(`button[aria-label="${label}"]`)
  )[0];
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function chooseFile(name = "Customer PO.pdf", content = "%PDF-1.7\n"): void {
  const input = document.querySelector<HTMLInputElement>(
    'input[aria-label="Customer PO PDF file"]'
  );
  if (!input) throw new Error("file input not found");
  const file = new File([content], name, { type: "application/pdf" });
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [file],
  });
  act(() => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

describe("customer PO PDF panel interactions (issue #472)", () => {
  it("loads the empty state and attaches a chosen PDF without touching the PO", async () => {
    stateMock.mockResolvedValueOnce(stateWith(null));
    attachMock.mockResolvedValueOnce(stateWith(1).Document);
    stateMock.mockResolvedValueOnce(stateWith(1));

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    expect(container.textContent).toContain("No customer PO PDF attached");

    chooseFile();
    await flush();

    expect(attachMock).toHaveBeenCalledTimes(1);
    // The real wire shape: a base64 string, not a numeric array. The constant
    // is the exact value the Go binding test pins for "%PDF-1.7\n".
    const payload = attachMock.mock.calls[0][0] as {
      PurchaseOrderID: number;
      Filename: string;
      DataBase64: string;
    };
    expect(payload.PurchaseOrderID).toBe(7);
    expect(payload.Filename).toBe("Customer PO.pdf");
    expect(payload.DataBase64).toBe("JVBERi0xLjcK");
    expect("Data" in payload).toBe(false);
    expect(stateMock).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain("Customer PO 1.pdf");
    expect(container.textContent).toContain("Version 1 of 1");
    expect(button("View customer PO PDF")).toBeTruthy();
    expect(button("Replace customer PO PDF")).toBeTruthy();
    expect(button("Remove customer PO PDF")).toBeTruthy();
  });

  it("replaces the current PDF and shows the retained version count", async () => {
    stateMock.mockResolvedValueOnce(stateWith(1));
    attachMock.mockResolvedValueOnce(stateWith(2).Document);
    stateMock.mockResolvedValueOnce(stateWith(2, [2, 1]));

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    expect(container.textContent).toContain("Version 1 of 1");

    chooseFile("Replacement.pdf");
    await flush();

    expect(attachMock).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("Customer PO 2.pdf");
    expect(container.textContent).toContain("Version 2 of 2");
  });

  it("ignores an initial state read that lands after a replacement was read", async () => {
    const delayedInitialRead = deferred<PurchaseOrderDocumentStateDTO>();
    stateMock.mockReturnValueOnce(delayedInitialRead.promise);
    attachMock.mockResolvedValueOnce(stateWith(2).Document);
    stateMock.mockResolvedValueOnce(stateWith(2, [2, 1]));

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    expect(container.textContent).toContain("Loading the customer PO PDF…");

    // A replacement is read while the panel's first read is still in flight.
    chooseFile("Replacement.pdf");
    await flush();
    expect(container.textContent).toContain("Customer PO 2.pdf");
    expect(container.textContent).toContain("Version 2 of 2");

    // The first read now answers with the document it replaced. It must not
    // overwrite the newer state, or the editor would be told the old document
    // is active again.
    await act(async () => {
      delayedInitialRead.resolve(stateWith(1));
      await Promise.resolve();
    });
    await flush();

    expect(container.textContent).toContain("Customer PO 2.pdf");
    expect(container.textContent).toContain("Version 2 of 2");
    expect(container.textContent).not.toContain("Customer PO 1.pdf");
  });

  it("asks before removing the current PDF and keeps it when told to", async () => {
    stateMock.mockResolvedValueOnce(stateWith(2, [2, 1]));

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    act(() => button("Remove customer PO PDF").click());

    const dialog = container.querySelector<HTMLElement>(
      '[role="alertdialog"][aria-label="Remove the customer PO PDF?"]'
    );
    expect(dialog).not.toBeNull();
    expect(removeMock).not.toHaveBeenCalled();

    act(() => button("Keep PDF", dialog!).click());
    expect(
      container.querySelector('[role="alertdialog"]')
    ).toBeNull();
    expect(removeMock).not.toHaveBeenCalled();
    expect(container.textContent).toContain("Customer PO 2.pdf");

    // Escape is the same decision as Keep PDF.
    act(() => button("Remove customer PO PDF").click());
    const again = container.querySelector<HTMLElement>('[role="alertdialog"]')!;
    act(() => {
      again.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true })
      );
    });
    expect(container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(removeMock).not.toHaveBeenCalled();
  });

  it("removes only after the confirmation and refreshes to the empty state", async () => {
    stateMock.mockResolvedValueOnce(stateWith(2, [2, 1]));
    removeMock.mockResolvedValueOnce(stateWith(2, [2, 1]).Document);
    stateMock.mockResolvedValueOnce(stateWith(null, [2, 1]));

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    act(() => button("Remove customer PO PDF").click());
    const dialog = container.querySelector<HTMLElement>('[role="alertdialog"]')!;
    await act(async () => {
      button("Remove PDF", dialog).click();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(removeMock).toHaveBeenCalledWith(7, true);
    expect(container.textContent).toContain("No customer PO PDF attached");
    expect(container.textContent).toContain("2 earlier versions are kept in the correction history.");
    expect(
      container.querySelector('[role="alertdialog"]')
    ).toBeNull();
  });

  it("shows a server validation refusal inline, including the unlocked-copy request", async () => {
    stateMock.mockResolvedValueOnce(stateWith(null));
    attachMock.mockRejectedValueOnce(
      new Error("the customer PO PDF is password-protected; an unlocked copy is required")
    );

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    chooseFile("locked.pdf");
    await flush();

    expect(container.textContent).toContain(
      "This PDF is password-protected. Attach an unlocked copy; Vera does not store PDF passwords."
    );
    expect(container.textContent).toContain("No customer PO PDF attached");
    expect(button("Attach customer PO PDF")).toBeTruthy();
  });

  it("opens the current PDF in a preview and closes it", async () => {
    stateMock.mockResolvedValueOnce(stateWith(1));
    contentMock.mockResolvedValueOnce({
      DataBase64: "JVBERi0xLjcK",
      ContentType: "application/pdf",
      Filename: "Customer PO 1.pdf",
      SHA256: "sha-1",
      Version: 1,
    });
    let previewBlob: Blob | null = null;
    const createObjectURL = vi.fn((blob: Blob) => {
      previewBlob = blob;
      return "blob:po-pdf";
    });
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: revokeObjectURL });

    act(() => root.render(<PurchaseOrderDocumentPanel purchaseOrderId={7} />));
    await flush();
    await act(async () => {
      button("View customer PO PDF").click();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(contentMock).toHaveBeenCalledWith(7);
    // The preview must hold the exact decoded bytes, not an empty array.
    expect(previewBlob).not.toBeNull();
    const previewBytes = new Uint8Array(await previewBlob!.arrayBuffer());
    expect(Array.from(previewBytes)).toEqual(
      Array.from(new TextEncoder().encode("%PDF-1.7\n"))
    );
    const frame = container.querySelector<HTMLIFrameElement>(
      'iframe[title="Customer PO PDF preview"]'
    );
    expect(frame?.src).toBe("blob:po-pdf");
    expect(container.textContent).toContain("Customer PO 1.pdf");

    act(() => button("Close PDF preview").click());
    expect(
      container.querySelector('iframe[title="Customer PO PDF preview"]')
    ).toBeNull();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:po-pdf");
  });
});
