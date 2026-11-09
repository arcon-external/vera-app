// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AdminDashboard } from "./AdminDashboard";
import { ToastProvider } from "../toast";

// Issue #494: the body follows the page the shell navigated to. With the
// duplicate tablist gone, the prop is the only driver, so rendering the same
// dashboard with a different tab must swap the panel. The real panel
// components render; only the Wails bindings are replaced at their module
// boundary, which is what proves the swap reached the right panel.

const { listClientsMock, listUsersMock } = vi.hoisted(() => ({
  listClientsMock: vi.fn(async (..._args: unknown[]) => []),
  listUsersMock: vi.fn(async (..._args: unknown[]) => []),
}));

vi.mock("@backend/backend/AdminAPI", () => ({
  ListClients: (...args: unknown[]) => listClientsMock(...args),
  ListUsers: (...args: unknown[]) => listUsersMock(...args),
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
  DownloadBackup: vi.fn(),
  GetCompanyLabels: vi.fn(async () => []),
  GetCompanyLogo: vi.fn(),
  GetCompanyProfile: vi.fn(),
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
  PickDirectory: vi.fn(),
  RefreshCompanyIdentity: vi.fn(),
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
  listClientsMock.mockClear();
  listUsersMock.mockClear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function renderTab(tab: "clients" | "team"): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <AdminDashboard tab={tab} />
      </ToastProvider>
    );
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("AdminDashboard body follows the page (issue #494)", () => {
  it("opens Clients first and swaps to Team when the page changes", async () => {
    await renderTab("clients");
    expect(document.querySelector("#clients-heading")).not.toBeNull();
    expect(listClientsMock).toHaveBeenCalledTimes(1);

    await renderTab("team");
    expect(document.querySelector("#users-heading")).not.toBeNull();
    expect(document.querySelector("#clients-heading")).toBeNull();
    expect(listUsersMock).toHaveBeenCalledTimes(1);
  });

  it("renders no Admin tablist of its own", async () => {
    await renderTab("clients");
    expect(document.querySelector('[aria-label="Admin sections"]')).toBeNull();
    expect(document.querySelector("[role=tablist]")).toBeNull();
    expect(document.querySelector("[role=tabpanel]")).toBeNull();
  });
});
