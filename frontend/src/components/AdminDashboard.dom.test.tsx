// @vitest-environment jsdom
import { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AdminDashboard, type AdminTabId } from "./AdminDashboard";
import { ToastProvider } from "../toast";

// Issue #491: Admin Dashboard tab switching. The real panel components render;
// only the Wails bindings are replaced at their module boundary. Clients and
// Team each load their own list through their own binding, which is what proves
// the tab switch reached the right panel rather than merely repainting the bar.

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

function Harness() {
  const [tab, setTab] = useState<AdminTabId>("clients");
  return (
    <ToastProvider>
      <AdminDashboard tab={tab} onTabChange={setTab} />
    </ToastProvider>
  );
}

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

function tab(label: string): HTMLButtonElement {
  const found = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="tab"]')
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`tab ${label} not found`);
  return found;
}

describe("AdminDashboard tab switching (issue #491)", () => {
  it("opens Clients first and switches to Team and back", async () => {
    await act(async () => {
      root.render(<Harness />);
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(document.querySelector("#clients-heading")).not.toBeNull();

    await act(async () => {
      tab("Team").click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(document.querySelector("#users-heading")).not.toBeNull();
    expect(document.querySelector("#clients-heading")).toBeNull();
    expect(listUsersMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      tab("Clients").click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(document.querySelector("#clients-heading")).not.toBeNull();
    expect(document.querySelector("#users-heading")).toBeNull();
  });
});
