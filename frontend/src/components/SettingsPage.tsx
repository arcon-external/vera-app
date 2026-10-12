import { useCallback, useEffect, useState } from "react";
import { ChangePassword, LogoutEverywhere, MyEmailChangeRequests, RequestDeletion, RequestEmailChange, UpdateProfile } from "@backend/backend/AuthAPI";
import { backend } from "@backend/models";
import {
  ClearCompanyLogo,
  DownloadBackup,
  GetCompanyLogo,
  GetCompanyProfile,
  GetCurrencyCatalogue,
  RestoreBackup,
  SetCompanyLogo,
  SetUserEmail,
  UpdateCompanyProfile,
} from "@backend/backend/AdminAPI";
import { PickDirectory, RefreshCompanyIdentity } from "@backend/backend/Quotation";
import { CheckForUpdates, Version } from "@backend/backend/UpdateAPI";
import {
  SHORTCUT_ACTIONS,
  bindingFromEvent,
  formatBinding,
  loadBindings,
  resetBindings,
  saveBindings,
  validateBindings,
  type ActionId,
  type Binding,
  type BindingMap,
} from "../shortcuts";
import { BrowserOpenURL } from "../../wailsjs/runtime";
import {
  EMPTY_THEME,
  extrasFromProfile,
  formFromProfile,
  profilePayload,
  themeFromProfile,
} from "../company";
import type { CompanyForm, CompanyTheme, StoredCompanyExtras } from "../company";
import type { CurrencyInfo } from "../format";
import { useToast } from "../toast";
import { CompanyGeneralPanel } from "./CompanyGeneralPanel";
import { CompanyThemePanel } from "./CompanyThemePanel";
import { TabBar, tabId, tabPanelId } from "./TabBar";
import type { TabItem } from "./TabBar";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";
import type { SignedInUser } from "./LoginPage";

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

// EXPORT_DIR_KEY is the Settings export default: a device-local folder the
// save dialog opens in. localStorage, never the server — a client-machine
// path is meaningless anywhere else, and exports must work offline.
export const EXPORT_DIR_KEY = "vera-export-dir";

export function SettingsPage({
  isAdmin,
  user,
  onProfileChanged,
  onCompanyChanged,
}: {
  isAdmin: boolean;
  user: SignedInUser;
  onProfileChanged: (user: SignedInUser) => void;
  onCompanyChanged?: () => void;
}) {
  const [section, setSection] = useState<"general" | "company" | "backups" | "keyboard">("general");
  const tabs = (
    [
      { id: "general", label: "General", adminOnly: false },
      { id: "company", label: "Company", adminOnly: true },
      { id: "backups", label: "Backups", adminOnly: true },
      { id: "keyboard", label: "Keyboard", adminOnly: false },
    ] as const
  ).filter((tab) => isAdmin || !tab.adminOnly);
  return (
    <section aria-labelledby="settings-heading" className="space-y-6">
      <div>
        <h1 id="settings-heading" className="text-display font-semibold text-ink-950">
          Settings
        </h1>
        <p className="mt-1.5 max-w-3xl text-body text-ink-500">
          {section === "company"
            ? "How the company appears across the app. Only admins see this section."
            : section === "backups"
              ? "Database safety nets. Only admins see this section."
              : section === "keyboard"
                ? "Your keyboard shortcuts. Stored on this device, editable anytime."
                : "Your own account. Team management lives on the Team page for admins."}
        </p>
      </div>

      <TabBar
        items={tabs.map((tab) => ({ id: tab.id, label: tab.label }))}
        active={section}
        onSelect={(id) => setSection(id as "general" | "company" | "backups" | "keyboard")}
        label="Settings sections"
      />

      {section === "company" && isAdmin ? (
        <CompanyProfileCard onSaved={onCompanyChanged} />
      ) : section === "backups" && isAdmin ? (
        <>
          <BackupCard />
          <RestoreCard />
        </>
      ) : section === "keyboard" ? (
        <KeyboardCard />
      ) : (
        <div className="grid items-stretch gap-6 lg:grid-cols-2 xl:grid-cols-3">
          <ProfileCard user={user} onProfileChanged={onProfileChanged} />
          <EmailCard user={user} isAdmin={isAdmin} />
          <PasswordCard />
          <SessionsCard />
          <UpdateCard />
          <ExportCard />
          <div className="lg:col-span-2 xl:col-span-3">
            <DeletionCard />
          </div>
        </div>
      )}
    </section>
  );
}

function ProfileCard({
  user,
  onProfileChanged,
}: {
  user: SignedInUser;
  onProfileChanged: (user: SignedInUser) => void;
}) {
  const [displayName, setDisplayName] = useState(user.DisplayName);
  const [preview, setPreview] = useState<string | null>(null);
  const [pendingFile, setPendingFile] = useState<{ base64: string; mime: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const pickFile = async (file: File | undefined) => {
    setError("");
    setPendingFile(null);
    setPreview(null);
    if (!file) return;
    if (file.size > 512 * 1024) {
      setError("That picture is over 512KB. Choose a smaller file.");
      return;
    }
    if (!["image/png", "image/jpeg", "image/gif", "image/webp"].includes(file.type)) {
      setError("Pictures must be PNG, JPEG, GIF, or WebP.");
      return;
    }
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(new Error("read failed"));
      reader.readAsDataURL(file);
    }).catch(() => {
      setError("We couldn't read that file.");
      return null;
    });
    if (!dataUrl) return;
    const match = /^data:(image\/[a-z+]+);base64,(.+)$/.exec(dataUrl);
    if (!match) {
      setError("We couldn't read that file.");
      return;
    }
    setPreview(dataUrl);
    setPendingFile({ base64: match[2], mime: match[1] });
  };

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const updated = (await UpdateProfile(
        displayName.trim() === user.DisplayName ? "" : displayName.trim(),
        pendingFile?.base64 ?? "",
        pendingFile?.mime ?? "",
        false
      )) as SignedInUser;
      onProfileChanged(updated);
      setDisplayName(updated.DisplayName);
      setPendingFile(null);
      setPreview(null);
      pushToast("success", "Profile updated.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't update your profile.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await UpdateProfile("", "", "", true);
      onProfileChanged({ ...user, AvatarURL: undefined });
      setPendingFile(null);
      setPreview(null);
      pushToast("success", "Profile picture removed.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't remove your picture.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  const shown = preview ?? user.AvatarURL ?? null;

  return (
    <Card className="p-5">
      <SectionHeading title="Profile" />
      <form onSubmit={save} className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <div className="flex flex-col items-center gap-2">
          <span className="flex h-20 w-20 items-center justify-center overflow-hidden rounded-full bg-accent-100 text-lead font-semibold text-accent-800" aria-hidden="true">
            {shown ? (
              <img src={shown} alt="" className="h-full w-full object-cover" />
            ) : (
              user.DisplayName.slice(0, 2).toUpperCase()
            )}
          </span>
          <label className="cursor-pointer text-caption font-medium text-accent-700 hover:underline">
            Choose picture
            <input
              type="file"
              accept="image/png,image/jpeg,image/gif,image/webp"
              className="sr-only"
              onChange={(event) => void pickFile(event.target.files?.[0])}
            />
          </label>
          {shown ? (
            <button
              type="button"
              onClick={() => void remove()}
              className="text-caption text-ink-400 hover:text-danger-600 hover:underline"
            >
              Remove
            </button>
          ) : null}
        </div>
        <div className="grid flex-1 gap-4">
          {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Display name</span>
            <input
              type="text"
              value={displayName}
              maxLength={80}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder="How your name appears"
              className={fieldClass}
            />
          </label>
          <div>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save profile"}
            </Button>
          </div>
        </div>
      </form>
      <p className="mt-3 text-caption leading-relaxed text-ink-400">
        Pictures must be PNG, JPEG, GIF, or WebP, under 512KB and 1024px per side.
      </p>
    </Card>
  );
}

function EmailCard({ user, isAdmin }: { user: SignedInUser; isAdmin: boolean }) {
  if (isAdmin) return <AdminEmailChanger user={user} />;
  return <StaffEmailRequest user={user} />;
}

function AdminEmailChanger({ user }: { user: SignedInUser }) {
  const [next, setNext] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const change = async () => {
    const email = next.trim();
    if (!email || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = (await SetUserEmail(user.ID, email)) as backend.EmailChangeResult;
      setNext("");
      setConfirming(false);
      pushToast(
        "success",
        `Sign-in email is now ${result.Email}. All sessions ended — sign in again with the new address.`
      );
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't change this email.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Email address" />
      <p className="max-w-2xl text-body text-ink-600">
        Signed in as <span className="font-medium text-ink-900">{user.Email}</span>.
        As an admin you can change it immediately — every session, including
        this one, ends at once.
      </p>
      {!confirming ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (next.trim()) {
              setError("");
              setConfirming(true);
            }
          }}
          className="mt-4 grid max-w-xl gap-3"
        >
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">New email</span>
            <input
              type="email"
              required
              value={next}
              onChange={(event) => setNext(event.target.value)}
              placeholder="you@company.com"
              className={fieldClass}
            />
          </label>
          <div>
            <Button type="submit" variant="primary" disabled={!next.trim()}>
              Change email
            </Button>
          </div>
        </form>
      ) : (
        <div className="mt-4 grid max-w-xl gap-3">
          {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
          <StatusBanner tone="danger">
            Change the sign-in email from{" "}
            <span className="font-medium">{user.Email}</span> to{" "}
            <span className="font-medium">{next.trim()}</span>? All sessions end
            immediately, including this one.
          </StatusBanner>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="danger" disabled={busy} onClick={() => void change()}>
              {busy ? "Changing…" : "Change email now"}
            </Button>
            <Button variant="ghost" onClick={() => setConfirming(false)}>
              Back
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
}

function StaffEmailRequest({ user }: { user: SignedInUser }) {
  const [proposed, setProposed] = useState("");
  const [rows, setRows] = useState<backend.EmailChangeRequest[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    try {
      const result = (await MyEmailChangeRequests()) as backend.EmailChangeRequest[];
      setRows(result);
    } catch {
      // Listing is best-effort; request errors surface on submit.
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const next = proposed.trim();
    if (!next || busy) return;
    setBusy(true);
    setError("");
    try {
      await RequestEmailChange(next);
      setProposed("");
      pushToast("success", "Request sent. An admin must approve it before anything changes.");
      await load();
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't send your request.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  const pending = rows.some((row) => row.Status === "pending");

  return (
    <Card className="p-5">
      <SectionHeading title="Email address" />
      <p className="max-w-2xl text-body text-ink-600">
        Signed in as <span className="font-medium text-ink-900">{user.Email}</span>.
        Changing it needs an admin's approval — you stay signed in until then.
      </p>
      <form onSubmit={submit} className="mt-4 grid max-w-xl gap-3">
        {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Proposed email</span>
          <input
            type="email"
            required
            value={proposed}
            onChange={(event) => setProposed(event.target.value)}
            placeholder="you@company.com"
            disabled={pending}
            className={fieldClass}
          />
        </label>
        <div>
          <Button type="submit" variant="secondary" disabled={busy || !proposed.trim() || pending}>
            {busy ? "Sending…" : pending ? "Request pending" : "Request email change"}
          </Button>
        </div>
      </form>
      {rows.length > 0 ? (
        <ul className="mt-4 divide-y divide-rule rounded-lg border border-rule">
          {rows.map((row) => (
            <li key={row.ID} className="flex flex-wrap items-center gap-2 px-4 py-2.5">
              <span className="min-w-0 flex-1 truncate text-body text-ink-800">
                {row.ProposedEmail}
              </span>
              <Badge tone={row.Status === "approved" ? "success" : row.Status === "rejected" ? "neutral" : "internal"}>
                {row.Status === "pending" ? "Pending" : row.Status === "approved" ? "Approved" : "Rejected"}
              </Badge>
            </li>
          ))}
        </ul>
      ) : null}
    </Card>
  );
}

const COMPANY_TABS: TabItem[] = [
  { id: "general", label: "General" },
  { id: "quotation", label: "Quotation" },
  { id: "portal", label: "Portal" },
];

export function CompanyProfileCard({ onSaved }: { onSaved?: () => void }) {
  const [companyTab, setCompanyTab] = useState<"general" | "quotation" | "portal">("general");
  const [form, setForm] = useState<CompanyForm>({
    legalName: "",
    address: "",
  });
  const [extras, setExtras] = useState<StoredCompanyExtras>({ phone: "", email: "", taxId: "", currency: "" });
  // The shipped currency catalogue. Fetched here, not in the panel: the
  // panel is a presentational slice by design (see company.ts) and owns no
  // loading state. Null while loading disables the picker; a failed fetch
  // also leaves null rather than blocking the profile load below.
  const [catalogue, setCatalogue] = useState<CurrencyInfo[] | null>(null);
  const [theme, setTheme] = useState<CompanyTheme>(EMPTY_THEME);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [logoPreview, setLogoPreview] = useState<string | null>(null);
  const [hasLogo, setHasLogo] = useState(false);
  const [logoBusy, setLogoBusy] = useState(false);
  const [logoMeta, setLogoMeta] = useState<string | null>(null);
  const { pushToast } = useToast();

  const loadLogo = useCallback(async () => {
    try {
      const logo = (await GetCompanyLogo()) as backend.CompanyLogo;
      if (!logo.LogoBase64) throw new Error("empty logo");
      const preview = `data:${logo.LogoMime};base64,${logo.LogoBase64}`;
      setLogoPreview(preview);
      setHasLogo(!logo.BuiltIn);
      const bytes = Math.floor((logo.LogoBase64.length * 3) / 4);
      const kind = (logo.LogoMime.split("/")[1] ?? "").toUpperCase() || "Image";
      const size =
        bytes >= 1024
          ? `${(bytes / 1024).toFixed(bytes >= 102400 ? 0 : 1)} KB`
          : `${bytes} B`;
      const dims = await new Promise<string>((resolve) => {
        const img = new Image();
        img.onload = () =>
          resolve(` · ${img.naturalWidth} × ${img.naturalHeight} px`);
        img.onerror = () => resolve("");
        img.src = preview;
      });
      setLogoMeta(
        logo.BuiltIn ? `${kind} · built in` : `${kind} · ${size}${dims}`
      );
    } catch {
      setLogoPreview(null);
      setHasLogo(false);
      setLogoMeta(null);
    }
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const profile = (await GetCompanyProfile()) as backend.CompanyProfile;
      setForm(formFromProfile(profile));
      setExtras(extrasFromProfile(profile));
      setTheme(themeFromProfile(profile));
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't load the company profile.");
      setError(message);
    } finally {
      setLoading(false);
    }
    try {
      setCatalogue((await GetCurrencyCatalogue()) as CurrencyInfo[]);
    } catch {
      // The picker stays disabled on its own; the profile above already loaded.
      setCatalogue(null);
    }
  }, []);

  useEffect(() => {
    void load();
    void loadLogo();
  }, [load, loadLogo]);

  const pickLogo = async (file: File | undefined) => {
    setError("");
    if (!file) return;
    if (file.size > 512 * 1024) {
      const message = "That logo is over 512KB. Choose a smaller file.";
      setError(message);
      pushToast("error", message);
      return;
    }
    if (!["image/png", "image/jpeg", "image/gif", "image/webp"].includes(file.type)) {
      const message = "Logo must be PNG, JPEG, GIF, or WebP.";
      setError(message);
      pushToast("error", message);
      return;
    }
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(new Error("read failed"));
      reader.readAsDataURL(file);
    }).catch(() => {
      setError("We couldn't read that file.");
      return null;
    });
    if (!dataUrl) return;
    const match = /^data:(image\/[a-z+]+);base64,(.+)$/.exec(dataUrl);
    if (!match) {
      setError("We couldn't read that file.");
      return;
    }
    if (logoBusy) return;
    setLogoBusy(true);
    try {
      await SetCompanyLogo(match[2], match[1]);
      await loadLogo();
      await RefreshCompanyIdentity();
      onSaved?.();
      pushToast("success", "Logo updated on future quotations.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't save this logo.");
      setError(message);
      pushToast("error", message);
    } finally {
      setLogoBusy(false);
    }
  };

  const removeLogo = async () => {
    if (logoBusy) return;
    setLogoBusy(true);
    setError("");
    try {
      await ClearCompanyLogo();
      await loadLogo();
      await RefreshCompanyIdentity();
      onSaved?.();
      pushToast("success", "Logo removed. Quotations use the built-in logo.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't remove the logo.");
      setError(message);
      pushToast("error", message);
    } finally {
      setLogoBusy(false);
    }
  };

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (loading) return;
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const updated = (await UpdateCompanyProfile(
        profilePayload({ form, extras, theme, hasLogo })
      )) as backend.CompanyProfile;
      setForm(formFromProfile(updated));
      setExtras(extrasFromProfile(updated));
      setTheme(themeFromProfile(updated));
      pushToast("success", "Company profile saved.");
      try {
        await RefreshCompanyIdentity();
      } catch {
        // Previews keep the previous header until the next sign-in;
        // the stored profile (and shell name) already updated above.
      }
      onSaved?.();
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't save the company profile.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  const companyTabs = (
    <TabBar
      items={COMPANY_TABS}
      active={companyTab}
      onSelect={(id) => setCompanyTab(id as "general" | "quotation" | "portal")}
      label="Company sections"
      idPrefix="company"
    />
  );

  // One <form> for both states: the panel swaps the skeleton for the live
  // inputs without remounting, so focus survives the load. Save stays
  // disabled until load() fills the record — a full-replace save has nothing
  // to send yet. (State ownership: see company.ts.)
  return (
    <form onSubmit={save}>
      {error ? (
        <div className="mb-6">
          <StatusBanner tone="danger">{error}</StatusBanner>
        </div>
      ) : null}
      {companyTabs}
      <div
        role="tabpanel"
        id={tabPanelId("company", companyTab)}
        aria-labelledby={tabId("company", companyTab)}
        className="mt-6"
      >
        {loading ? (
          <Card className="p-5">
            <SectionHeading title="Company profile" />
            <div className="h-48 animate-pulse rounded-xl bg-paper-muted" aria-label="Loading company profile" />
          </Card>
        ) : companyTab === "general" ? (
          <CompanyGeneralPanel
            form={form}
            onFieldChange={(field, value) => setForm((current) => ({ ...current, [field]: value }))}
            logoPreview={logoPreview}
            hasLogo={hasLogo}
            logoBusy={logoBusy}
            logoMeta={logoMeta}
            onPickLogo={pickLogo}
            onRemoveLogo={removeLogo}
            currency={extras.currency}
            catalogue={catalogue}
            onCurrencyChange={(code) => setExtras((current) => ({ ...current, currency: code }))}
          />
        ) : (
          <CompanyThemePanel
            section={companyTab === "quotation" ? "Quotation" : "Portal"}
            theme={theme}
            onChange={(key, next) => setTheme((current) => ({ ...current, [key]: next }))}
          />
        )}
      </div>
      <div className="mt-6">
        <Button type="submit" variant="primary" disabled={loading || busy}>
          {busy ? "Saving…" : "Save company profile"}
        </Button>
      </div>
    </form>
  );
}

function PasswordCard() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (next !== confirm) {
      setError("The new passwords do not match.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await ChangePassword(current, next);
      setCurrent("");
      setNext("");
      setConfirm("");
      pushToast("success", "Password changed. Other devices were signed out.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't change your password.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Password" />
      <form onSubmit={submit} className="grid max-w-xl gap-4">
        {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Current password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(event) => setCurrent(event.target.value)}
            placeholder="Required unless this is your first setup"
            className={fieldClass}
          />
        </label>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">New password</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              minLength={10}
              value={next}
              onChange={(event) => setNext(event.target.value)}
              placeholder="At least 10 characters"
              className={fieldClass}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Confirm new password</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              placeholder="Repeat it"
              className={fieldClass}
            />
          </label>
        </div>
        <div>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Changing…" : "Change password"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

function BackupCard() {
  const [busy, setBusy] = useState(false);
  const { pushToast } = useToast();

  const download = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const path = (await DownloadBackup()) as string;
      pushToast("success", `Backup saved to ${path}.`);
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't download the backup."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Database backup" />
      <p className="max-w-2xl text-body text-ink-600">
        Download the newest nightly database backup to this machine. To
        restore one, use the restore section below — it takes a safety
        snapshot first and signs everyone out.
      </p>
      <div className="mt-4">
        <Button variant="secondary" onClick={() => void download()} disabled={busy}>
          {busy ? "Preparing…" : "Download latest backup"}
        </Button>
      </div>
    </Card>
  );
}

interface RestoreOutcome {
  SafetySnapshot: string;
  Quotations: number;
  Revisions: number;
}

function RestoreCard() {
  const [confirmText, setConfirmText] = useState("");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<RestoreOutcome | null>(null);
  const { pushToast } = useToast();
  const armed = confirmText.trim().toUpperCase() === "RESTORE";

  const restore = async () => {
    if (busy || !armed) return;
    setBusy(true);
    setOutcome(null);
    try {
      const result = (await RestoreBackup()) as RestoreOutcome;
      setOutcome(result);
      setConfirmText("");
      pushToast(
        "success",
        `Restore complete: ${result.Quotations} quotations, ${result.Revisions} revisions. Everyone has been signed out — sign in again.`
      );
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't restore the backup."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="border-danger-200 p-5">
      <SectionHeading title="Restore database" />
      <StatusBanner tone="danger">
        Restoring replaces the entire database with the selected backup. A
        safety snapshot of the current state is taken first, and{" "}
        <span className="font-semibold">everyone is signed out immediately</span>{" "}
        — including you. Coordinate with the team before proceeding.
      </StatusBanner>
      <div className="mt-4 grid max-w-xl gap-3">
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">
            Type RESTORE to arm the restore button
          </span>
          <input
            type="text"
            value={confirmText}
            onChange={(event) => setConfirmText(event.target.value)}
            placeholder="RESTORE"
            autoComplete="off"
            className={fieldClass}
          />
        </label>
        <div>
          <Button variant="danger" onClick={() => void restore()} disabled={busy || !armed}>
            {busy ? "Restoring… (this takes a while)" : "Select backup and restore"}
          </Button>
        </div>
        {outcome ? (
          <p className="text-caption leading-relaxed text-ink-500">
            Restored {outcome.Quotations} quotations and {outcome.Revisions} revisions.
            Safety snapshot: <span className="font-medium text-ink-900">{outcome.SafetySnapshot}</span>.
            Sign in again to continue.
          </p>
        ) : null}
      </div>
    </Card>
  );
}

function SessionsCard() {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const { pushToast } = useToast();

  const confirm = async () => {
    setBusy(true);
    try {
      await LogoutEverywhere();
      pushToast("success", "Signed out everywhere. Sign in again to continue.");
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't sign out everywhere."));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Sessions" />
      <p className="max-w-2xl text-body text-ink-600">
        Ends every session for this account on all devices, including this one.
        You will be asked to sign in again.
      </p>
      <div className="mt-4">
        {confirming ? (
          <span className="flex flex-wrap items-center gap-2">
            <span className="text-caption text-ink-600">Sign out everywhere, including here?</span>
            <Button variant="danger" size="sm" disabled={busy} onClick={() => void confirm()}>
              Sign out everywhere
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>
              Keep me signed in
            </Button>
          </span>
        ) : (
          <Button variant="secondary" onClick={() => setConfirming(true)}>
            Sign out everywhere
          </Button>
        )}
      </div>
    </Card>
  );
}

function UpdateCard() {
  const [current, setCurrent] = useState("");
  const [info, setInfo] = useState<backend.UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  useEffect(() => {
    let cancelled = false;
    Version()
      .then((version) => {
        if (!cancelled) setCurrent(String(version));
      })
      .catch(() => {
        if (!cancelled) setCurrent("");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const check = async () => {
    if (checking) return;
    setChecking(true);
    setError("");
    try {
      const result = (await CheckForUpdates()) as backend.UpdateInfo;
      setInfo(result);
      if (result.Current) setCurrent(result.Current);
      if (!result.UpdateAvailable) {
        pushToast("success", "You're on the latest version.");
      }
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't check for updates.");
      setError(/reach|network|fetch|failed|econn|timeout/i.test(message)
        ? "You're offline. Connect and try again."
        : message);
    } finally {
      setChecking(false);
    }
  };

  const openTarget = (url: string) => {
    try {
      if (url) BrowserOpenURL(url);
    } catch {
      // Opening the browser must never break Settings.
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="App updates" />
      {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
      <dl className="grid gap-2 text-body">
        <div className="flex items-baseline justify-between gap-4">
          <dt className="text-ink-500">Current version</dt>
          <dd className="tnum font-medium text-ink-900">{current || "dev"}</dd>
        </div>
        {info ? (
          <>
            <div className="flex items-baseline justify-between gap-4">
              <dt className="text-ink-500">Latest version</dt>
              <dd className="tnum font-medium text-ink-900">{info.Latest || "unknown"}</dd>
            </div>
            <div className="flex items-baseline justify-between gap-4">
              <dt className="text-ink-500">Status</dt>
              <dd className="text-right font-medium text-ink-900">
                {info.UpdateAvailable
                  ? info.Required
                    ? `Update required — ${info.Latest} is the minimum supported.`
                    : `${info.Latest} is available.`
                  : "You're up to date."}
              </dd>
            </div>
          </>
        ) : null}
      </dl>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={() => void check()} disabled={checking}>
          {checking ? "Checking…" : info ? "Check again" : "Check for updates"}
        </Button>
        {info?.UpdateAvailable && info.DownloadURL ? (
          <Button variant="primary" onClick={() => openTarget(info.DownloadURL)}>
            Download update
          </Button>
        ) : null}
        {info?.UpdateAvailable && !info.DownloadURL && info.NotesURL ? (
          <Button variant="primary" onClick={() => openTarget(info.NotesURL)}>
            See what's new
          </Button>
        ) : null}
      </div>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        Vera checks once per launch. Download the installer for your system —
        it handles the rest.
      </p>
    </Card>
  );
}

function ExportCard() {
  const [dir, setDir] = useState(() => {
    try {
      return window.localStorage.getItem(EXPORT_DIR_KEY) ?? "";
    } catch {
      return "";
    }
  });
  const [busy, setBusy] = useState(false);
  const { pushToast } = useToast();

  const browse = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const picked = (await PickDirectory()) as string;
      if (picked) setDir(picked);
    } catch {
      // Cancelled or unavailable: keep whatever is typed.
    } finally {
      setBusy(false);
    }
  };

  const save = () => {
    const value = dir.trim();
    try {
      if (value) window.localStorage.setItem(EXPORT_DIR_KEY, value);
      else window.localStorage.removeItem(EXPORT_DIR_KEY);
    } catch {
      pushToast("error", "We couldn't save this setting on this device.");
      return;
    }
    pushToast("success", value ? "Export folder saved." : "Export folder cleared.");
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Export folder" />
      <p className="max-w-2xl text-body text-ink-600">
        The starting folder of the export save dialog. Leave empty to use the
        system default.
      </p>
      <div className="mt-3 grid gap-2">
        <input
          type="text"
          value={dir}
          onChange={(event) => setDir(event.target.value)}
          placeholder="/Users/you/Documents/Quotations"
          spellCheck={false}
          autoComplete="off"
          aria-label="Default export folder"
          className={`${fieldClass} font-mono`}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => void browse()} disabled={busy}>
            {busy ? "Browsing…" : "Browse…"}
          </Button>
          <Button variant="primary" size="sm" onClick={save}>
            Save folder
          </Button>
          {dir ? (
            <button
              type="button"
              onClick={() => setDir("")}
              className="text-caption text-ink-400 hover:text-danger-600 hover:underline"
            >
              Clear
            </button>
          ) : null}
        </div>
      </div>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        Stored on this device only. Exports still ask where to save every time;
        a missing folder falls back to the system default.
      </p>
    </Card>
  );
}

function KeyboardCard() {
  const [bindings, setBindings] = useState<Record<ActionId, Binding>>(() => loadBindings());
  const [recording, setRecording] = useState<ActionId | null>(null);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const persist = (next: Record<ActionId, Binding>) => {
    const overrides: BindingMap = {};
    for (const action of SHORTCUT_ACTIONS) {
      const current = next[action.id];
      const def = action.default;
      if (
        current.mod !== def.mod ||
        current.ctrl !== def.ctrl ||
        current.shift !== def.shift ||
        current.alt !== def.alt ||
        current.key !== def.key
      ) {
        overrides[action.id] = { ...current };
      }
    }
    try {
      saveBindings(overrides);
    } catch {
      pushToast("error", "We couldn't save this setting on this device.");
      return;
    }
    setBindings(next);
  };

  const capture = (id: ActionId, event: React.KeyboardEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (event.key === "Escape") {
      setRecording(null);
      return;
    }
    const binding = bindingFromEvent(event.nativeEvent);
    if (!binding) return;
    const next = { ...bindings, [id]: binding };
    const conflict = validateBindings(next);
    if (conflict) {
      setError(conflict);
      return;
    }
    setError("");
    setRecording(null);
    persist(next);
    pushToast("success", "Shortcut updated. It works immediately.");
  };

  const resetOne = (id: ActionId) => {
    const def = SHORTCUT_ACTIONS.find((action) => action.id === id);
    if (!def) return;
    setError("");
    persist({ ...bindings, [id]: { ...def.default } });
  };

  const resetAll = () => {
    resetBindings();
    setError("");
    setRecording(null);
    setBindings(loadBindings());
    pushToast("success", "Shortcuts reset to defaults.");
  };

  const groups: { title: string; ids: ActionId[] }[] = [
    { title: "Editor", ids: ["exportPdf", "prepare", "newQuotation", "gridUp", "gridDown", "gridLeft", "gridRight", "gridEnter", "gridEnterBack"] },
    { title: "Pages", ids: ["gotoEditor", "gotoArchive", "gotoClients", "gotoTeam", "gotoSettings", "toggleHelp"] },
    { title: "Archive", ids: ["archiveUp", "archiveDown", "archiveOpen"] },
  ];

  return (
    <Card className="p-5">
      <SectionHeading title="Keyboard" />
      {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
      <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
        {groups.map((group) => (
          <div key={group.title}>
            <h3 className="mb-2 text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              {group.title}
            </h3>
            <dl className="grid gap-3">
              {group.ids.map((id) => {
                const action = SHORTCUT_ACTIONS.find((a) => a.id === id);
                if (!action) return null;
                const customized =
                  JSON.stringify(bindings[id]) !== JSON.stringify(action.default);
                return (
                  <div key={id} className="flex items-center justify-between gap-2 text-small">
                    <dt className="min-w-0 text-ink-700">
                      {action.label}
                      {action.hint ? (
                        <span className="mt-0.5 block text-caption leading-snug text-ink-400">{action.hint}</span>
                      ) : null}
                    </dt>
                    <dd className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        onClick={() => {
                          setError("");
                          setRecording(id);
                        }}
                        onKeyDown={(event) => {
                          if (recording === id) capture(id, event);
                        }}
                        aria-label={`Rebind ${action.label}. Currently ${formatBinding(bindings[id])}`}
                        className={`tnum rounded-md border px-2 py-1 font-mono text-caption transition-colors duration-150 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                          recording === id
                            ? "border-accent-500 bg-accent-50 text-accent-700"
                            : "border-rule bg-paper-subtle text-ink-800 hover:border-ink-300"
                        }`}
                      >
                        {recording === id ? "Press keys…" : formatBinding(bindings[id])}
                      </button>
                      {customized ? (
                        <button
                          type="button"
                          onClick={() => resetOne(id)}
                          aria-label={`Reset ${action.label} to default`}
                          className="rounded px-1 text-caption text-ink-400 hover:text-ink-700 hover:underline"
                        >
                          Reset
                        </button>
                      ) : null}
                    </dd>
                  </div>
                );
              })}
            </dl>
          </div>
        ))}
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" onClick={resetAll}>
          Reset all to defaults
        </Button>
        <span className="text-caption text-ink-400">
          Stored on this device only. Enter always moves down in the grid;
          everything else rebinds above.
        </span>
      </div>
    </Card>
  );
}

function DeletionCard() {  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [requested, setRequested] = useState(false);
  const { pushToast } = useToast();

  const confirm = async () => {
    setBusy(true);
    try {
      await RequestDeletion(true);
      setRequested(true);
      pushToast("success", "Deletion requested. An admin must approve it.");
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't record your request."));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  };

  return (
    <Card className="border-danger-200 p-5">
      <SectionHeading title="Delete account" />
      {requested ? (
        <StatusBanner tone="info">
          Your request is pending. An admin must approve it; your quotations and
          history stay intact either way.
        </StatusBanner>
      ) : (
        <>
          <p className="max-w-2xl text-body text-ink-600">
            Request deletion of your account. An admin reviews and approves it —
            nothing is deleted automatically, and your past quotations are kept
            for the archive.
          </p>
          <div className="mt-4">
            {confirming ? (
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-caption text-ink-600">
                  Ask an admin to delete your account?
                </span>
                <Button variant="danger" size="sm" disabled={busy} onClick={() => void confirm()}>
                  Request deletion
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>
                  Keep account
                </Button>
              </span>
            ) : (
              <Button variant="danger" onClick={() => setConfirming(true)}>
                Request account deletion
              </Button>
            )}
          </div>
        </>
      )}
    </Card>
  );
}
