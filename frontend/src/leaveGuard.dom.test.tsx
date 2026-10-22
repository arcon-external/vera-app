// @vitest-environment jsdom
import { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./components/AppShell";
import { AdminDashboard, adminPageForTab, adminTabForPage } from "./components/AdminDashboard";
import { PurchaseOrderDiscardPrompt } from "./components/PurchaseOrderEditor";
import { usePurchaseOrderLeaveGuard, type AppPage } from "./poLeaveGuard";
import { ToastProvider } from "./toast";

// Issue #491: leaving a dirty Quotation editor or a dirty Company tab asks
// first. The harness renders the real shell, the real guard and the real
// discard modal; only the desktop bindings are replaced. The Company case also
// proves the dirty flag crosses from CompanyProfileCard to App through
// onDirtyChange, rather than being assumed from a source scan.

const { profileMock } = vi.hoisted(() => ({
  profileMock: vi.fn(async (..._args: unknown[]) => ({})),
}));

vi.mock("@backend/backend/AdminAPI", () => ({
  ListClients: vi.fn(async () => []),
  ListUsers: vi.fn(async () => []),
  ListDeletionRequests: vi.fn(async () => []),
  ListEmailChangeRequests: vi.fn(async () => []),
  CreateClient: vi.fn(),
  DeleteClient: vi.fn(),
  UpdateClient: vi.fn(),
  CreateUser: vi.fn(),
  DecideDeletion: vi.fn(),
  DecideEmailChange: vi.fn(),
  DeleteUser: vi.fn(),
  IssuePasswordReset: vi.fn(),
  SetUserActive: vi.fn(),
  SetUserEmail: vi.fn(),
  SetUserRole: vi.fn(),
  ClearCompanyLogo: vi.fn(),
  DownloadBackup: vi.fn(async () => "/tmp/backup.sql"),
  GetCompanyLabels: vi.fn(async () => []),
  GetCompanyLogo: vi.fn(async () => ({ LogoBase64: "", LogoMime: "", BuiltIn: true })),
  GetCompanyProfile: (...args: unknown[]) => profileMock(...args),
  GetCompanyUnits: vi.fn(async () => []),
  GetCurrencyCatalogue: vi.fn(async () => []),
  RestoreBackup: vi.fn(),
  SetCompanyLogo: vi.fn(),
  UpdateCompanyGeneral: vi.fn(),
  UpdateCompanyLabels: vi.fn(),
  UpdateCompanyUnits: vi.fn(),
  UpdatePortalTheme: vi.fn(),
  UpdateQuotationTheme: vi.fn(),
}));
vi.mock("@backend/backend/AuthAPI", () => ({
  ChangePassword: vi.fn(),
  LogoutEverywhere: vi.fn(),
  MyEmailChangeRequests: vi.fn(async () => []),
  RequestDeletion: vi.fn(),
  RequestEmailChange: vi.fn(),
  UpdateProfile: vi.fn(),
}));
vi.mock("@backend/backend/Quotation", () => ({
  PickDirectory: vi.fn(async () => ""),
  RefreshCompanyIdentity: vi.fn(async () => {}),
}));
vi.mock("@backend/backend/UpdateAPI", () => ({
  CheckForUpdates: vi.fn(),
  Version: vi.fn(async () => "v0.0.0"),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  profileMock.mockClear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function tab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const inNav = Array.from(
    nav?.querySelectorAll<HTMLButtonElement>("button") ?? []
  ).find((node) => node.textContent?.trim() === label);
  if (inNav !== undefined) return inNav;
  const inTablist = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="tab"]')
  ).find((node) => node.textContent?.trim() === label);
  if (inTablist === undefined) throw new Error(`no tab labelled ${label}`);
  return inTablist;
}

function prompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

function QuotationHarness() {
  const [page, setPage] = useState<AppPage>("create");
  const [dirty, setDirty] = useState(true);
  const leave = usePurchaseOrderLeaveGuard(page, setPage, {
    dirtyPages: { create: dirty },
    onDiscard: () => setDirty(false),
  });
  return (
    <>
      <AppShell
        page={page}
        inert={leave.pendingPage !== null}
        onNavigate={leave.request}
      >
        <div data-testid={`page-${page}`}>Quotation {page}</div>
      </AppShell>
      {leave.pendingPage !== null ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={leave.keepEditing}
          onDiscard={leave.discard}
          returnFocus={leave.pendingReturnFocus}
          description={leave.pendingDescription}
        />
      ) : null}
    </>
  );
}

function CompanyHarness() {
  const [page, setPage] = useState<AppPage>("admin-company");
  const [dirty, setDirty] = useState(false);
  const leave = usePurchaseOrderLeaveGuard(page, setPage, {
    dirtyPages: { "admin-company": dirty },
    onDiscard: () => setDirty(false),
  });
  return (
    <>
      <AppShell
        page={page}
        inert={leave.pendingPage !== null}
        onNavigate={leave.request}
      >
        <AdminDashboard
          tab={adminTabForPage(page)}
          onTabChange={(tab) => leave.request(adminPageForTab(tab))}
          onCompanyDirtyChange={setDirty}
        />
      </AppShell>
      {leave.pendingPage !== null ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={leave.keepEditing}
          onDiscard={leave.discard}
          returnFocus={leave.pendingReturnFocus}
          description={leave.pendingDescription}
        />
      ) : null}
    </>
  );
}

describe("Quotation exercise: leaving unsaved work asks (issue #491)", () => {
  it("asks before Drafts unmounts the editor, and Keep editing stays", async () => {
    await act(async () => {
      root.render(<QuotationHarness />);
    });
    await act(async () => {
      tab("Drafts").click();
    });

    expect(prompt()).not.toBeNull();
    expect(document.body.textContent).toContain(
      "The quotation in the editor has unsaved changes."
    );
    expect(document.querySelector('[data-testid="page-drafts"]')).toBeNull();

    await act(async () => {
      button("Keep editing").click();
    });
    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-create"]')).not.toBeNull();
  });

  it("Discard changes navigates to the requested tab", async () => {
    await act(async () => {
      root.render(<QuotationHarness />);
    });
    await act(async () => {
      tab("Drafts").click();
    });
    await act(async () => {
      button("Discard changes").click();
    });

    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-drafts"]')).not.toBeNull();
  });
});

describe("Admin/Company exercise: leaving unsaved edits asks (issue #491)", () => {
  it("asks before Backups unmounts an edited company profile", async () => {
    await act(async () => {
      root.render(
        <ToastProvider>
          <CompanyHarness />
        </ToastProvider>
      );
    });
    await flush();

    const legalName = document.querySelector<HTMLInputElement>(
      'input[placeholder="PT Arcon Penta Persada"]'
    );
    if (legalName === null) throw new Error("legal name field not found");
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(legalName, "PT Edited");
      legalName.dispatchEvent(new Event("input", { bubbles: true }));
    });

    await act(async () => {
      tab("Backups").click();
    });
    expect(prompt()).not.toBeNull();
    expect(document.body.textContent).toContain(
      "The company profile has unsaved changes."
    );
    expect(document.querySelector("#admin-panel-company")).not.toBeNull();

    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(prompt()).toBeNull();
    expect(document.body.textContent).toContain("Download latest backup");
  });
});
