import { useCallback, useEffect, useState } from "react";
import { ChangePassword, LogoutEverywhere, MyEmailChangeRequests, RequestDeletion, RequestEmailChange, UpdateProfile } from "@backend/backend/AuthAPI";
import { backend } from "@backend/models";
import {
  ClearCompanyLogo,
  DownloadBackup,
  GetCompanyLogo,
  GetCompanyProfile,
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
import { useToast } from "../toast";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";
import type { SignedInUser } from "./LoginPage";

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

// EXPORT_DIR_KEY is the Settings export default: a device-local folder the
// save dialog opens in. localStorage, never the server — a client-machine
// path is meaningless anywhere else, and exports must work offline.
export const EXPORT_DIR_KEY = "vera-export-dir";

const fieldClass =
  "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

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

      <div
        role="tablist"
        aria-label="Settings sections"
        className="inline-flex rounded-lg border border-rule bg-paper-muted p-0.5"
      >
        {tabs.map((tab) => {
            const selected = section === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={selected}
                onClick={() => setSection(tab.id)}
                className={`min-h-8 rounded-md px-4 py-1.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                  selected
                    ? "bg-white text-ink-950 shadow-card"
                    : "text-ink-500 hover:text-ink-800"
                }`}
              >
                {tab.label}
              </button>
            );
          })}
        </div>

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
        <div className="grid items-start gap-6 lg:grid-cols-2 xl:grid-cols-3">
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

type CompanyForm = {
  legalName: string;
  address: string;
};

// Stored-but-unedited profile values. Phone, email, and tax ID print
// nowhere, so they have no inputs — but they still round-trip verbatim so
// saving never wipes them.
type StoredCompanyExtras = {
  phone: string;
  email: string;
  taxId: string;
};

type CompanyTheme = {
  qDeep: string;
  qAccent: string;
  qSoft: string;
  qInk: string;
  pDeep: string;
  pAccent: string;
  pSoft: string;
  pInk: string;
};

const EMPTY_THEME: CompanyTheme = {
  qDeep: "",
  qAccent: "",
  qSoft: "",
  qInk: "",
  pDeep: "",
  pAccent: "",
  pSoft: "",
  pInk: "",
};

// Baked defaults mirror the migration seeds: shown when the stored value is
// empty, so the picker never renders an invalid color.
const DEFAULT_THEME: CompanyTheme = {
  qDeep: "#1A237E",
  qAccent: "#284AF3",
  qSoft: "#D6E4FF",
  qInk: "#1a1a1a",
  pDeep: "#1A237E",
  pAccent: "#284AF3",
  pSoft: "#D6E4FF",
  pInk: "#1a1a1a",
};

const THEME_FIELDS: {
  key: keyof CompanyTheme;
  section: string;
  label: string;
  hint: string;
}[] = [
  { key: "qDeep", section: "Quotation", label: "Deep", hint: "Headings, table headers, subject label, buttons, top ribbon on quotations" },
  { key: "qAccent", section: "Quotation", label: "Accent", hint: "Company name, links and highlights on quotations" },
  { key: "qSoft", section: "Quotation", label: "Soft", hint: "Card and section backgrounds on quotations" },
  { key: "qInk", section: "Quotation", label: "Ink", hint: "Body text on quotations" },
  { key: "pDeep", section: "Portal", label: "Deep", hint: "Ribbon, headings, buttons and eyebrow on the verification page" },
  { key: "pAccent", section: "Portal", label: "Accent", hint: "Issuer name, links and button hover on the verification page" },
  { key: "pSoft", section: "Portal", label: "Soft", hint: "Reserved section backgrounds on the verification page" },
  { key: "pInk", section: "Portal", label: "Ink", hint: "Body text on the verification page" },
];

function CompanyProfileCard({ onSaved }: { onSaved?: () => void }) {
  const [form, setForm] = useState<CompanyForm>({
    legalName: "",
    address: "",
  });
  const [extras, setExtras] = useState<StoredCompanyExtras>({ phone: "", email: "", taxId: "" });
  const [theme, setTheme] = useState<CompanyTheme>(EMPTY_THEME);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [logoPreview, setLogoPreview] = useState<string | null>(null);
  const [hasLogo, setHasLogo] = useState(false);
  const [logoBusy, setLogoBusy] = useState(false);
  const { pushToast } = useToast();

  const loadLogo = useCallback(async () => {
    try {
      const logo = (await GetCompanyLogo()) as backend.CompanyLogo;
      setLogoPreview(`data:${logo.LogoMime};base64,${logo.LogoBase64}`);
      setHasLogo(true);
    } catch {
      setLogoPreview(null);
      setHasLogo(false);
    }
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const profile = (await GetCompanyProfile()) as backend.CompanyProfile;
      setForm({
        legalName: profile.LegalName ?? "",
        address: profile.Address ?? "",
      });
      setExtras({
        phone: profile.Phone ?? "",
        email: profile.Email ?? "",
        taxId: profile.TaxID ?? "",
      });
      setTheme({
        qDeep: profile.ThemeQDeep ?? "",
        qAccent: profile.ThemeQAccent ?? "",
        qSoft: profile.ThemeQSoft ?? "",
        qInk: profile.ThemeQInk ?? "",
        pDeep: profile.ThemePDeep ?? "",
        pAccent: profile.ThemePAccent ?? "",
        pSoft: profile.ThemePSoft ?? "",
        pInk: profile.ThemePInk ?? "",
      });
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't load the company profile.");
      setError(message);
    } finally {
      setLoading(false);
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
      const message = "That artwork is over 512KB. Choose a smaller file.";
      setError(message);
      pushToast("error", message);
      return;
    }
    if (!["image/png", "image/jpeg", "image/gif", "image/webp"].includes(file.type)) {
      const message = "Artwork must be PNG, JPEG, GIF, or WebP.";
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
      const message = errorMessage(err, "We couldn't save this artwork.");
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
      pushToast("success", "Logo removed. Quotations use the built-in artwork.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't remove the artwork.");
      setError(message);
      pushToast("error", message);
    } finally {
      setLogoBusy(false);
    }
  };

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const updated = (await UpdateCompanyProfile({
        LegalName: form.legalName.trim(),
        Address: form.address.trim(),
        Phone: extras.phone,
        Email: extras.email,
        TaxID: extras.taxId,
        HasLogo: hasLogo,
        ThemeQDeep: theme.qDeep.trim(),
        ThemeQAccent: theme.qAccent.trim(),
        ThemeQSoft: theme.qSoft.trim(),
        ThemeQInk: theme.qInk.trim(),
        ThemePDeep: theme.pDeep.trim(),
        ThemePAccent: theme.pAccent.trim(),
        ThemePSoft: theme.pSoft.trim(),
        ThemePInk: theme.pInk.trim(),
      })) as backend.CompanyProfile;
      setForm({
        legalName: updated.LegalName ?? "",
        address: updated.Address ?? "",
      });
      setExtras({
        phone: updated.Phone ?? "",
        email: updated.Email ?? "",
        taxId: updated.TaxID ?? "",
      });
      setTheme({
        qDeep: updated.ThemeQDeep ?? "",
        qAccent: updated.ThemeQAccent ?? "",
        qSoft: updated.ThemeQSoft ?? "",
        qInk: updated.ThemeQInk ?? "",
        pDeep: updated.ThemePDeep ?? "",
        pAccent: updated.ThemePAccent ?? "",
        pSoft: updated.ThemePSoft ?? "",
        pInk: updated.ThemePInk ?? "",
      });
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

  const set = (field: keyof CompanyForm) => (event: React.ChangeEvent<HTMLInputElement>) =>
    setForm((current) => ({ ...current, [field]: event.target.value }));

  if (loading) {
    return (
      <Card className="p-5">
        <SectionHeading title="Company profile" />
        <div className="h-48 animate-pulse rounded-xl bg-paper-muted" aria-label="Loading company profile" />
      </Card>
    );
  }

  return (
    <form onSubmit={save}>
      {error ? (
        <div className="mb-6">
          <StatusBanner tone="danger">{error}</StatusBanner>
        </div>
      ) : null}
      <div className="grid items-start gap-6 lg:grid-cols-2">
        <Card className="p-5">
          <SectionHeading title="Company profile" />
          <div className="grid gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Legal name</span>
            <input
              type="text"
              required
              maxLength={160}
              value={form.legalName}
              onChange={set("legalName")}
              placeholder="PT Arcon Penta Persada"
              className={fieldClass}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Address</span>
            <input
              type="text"
              value={form.address}
              onChange={set("address")}
              placeholder="Street, city, province"
              className={fieldClass}
            />
          </label>
          <div>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save company profile"}
            </Button>
          </div>
          </div>
          <p className="mt-3 text-caption leading-relaxed text-ink-400">
            Only admins see this section. The quotation header prints this name
            and address on every new quotation.
          </p>
        </Card>
          {(["Quotation", "Portal"] as const).map((section) => (
            <Card key={section} className="p-5">
              <SectionHeading title={`${section} colors`} />
              <div className="grid grid-cols-2 gap-3">
                {THEME_FIELDS.filter((field) => field.section === section).map((field) => {
                  const value = theme[field.key].trim() || DEFAULT_THEME[field.key];
                  const setThemeValue = (next: string) =>
                    setTheme((current) => ({ ...current, [field.key]: next }));
                  return (
                    <label
                      key={field.key}
                      className="flex flex-col gap-1.5"
                      title={`${field.label}: ${field.hint}`}
                    >
                      <span className="text-caption font-medium text-ink-600">
                        {field.label}
                      </span>
                      <span className="flex items-center gap-2">
                        <input
                          type="color"
                          value={/^#[0-9a-fA-F]{6}$/.test(value) ? value : DEFAULT_THEME[field.key]}
                          onChange={(event) => setThemeValue(event.target.value)}
                          aria-label={`${section} ${field.label} color picker`}
                          className="h-9 w-10 shrink-0 cursor-pointer rounded-md border border-rule bg-white p-1"
                        />
                        <input
                          type="text"
                          value={theme[field.key]}
                          onChange={(event) => setThemeValue(event.target.value)}
                          placeholder={DEFAULT_THEME[field.key]}
                          spellCheck={false}
                          autoComplete="off"
                          aria-label={`${section} ${field.label} hex value`}
                          className={`${fieldClass} font-mono`}
                        />
                      </span>
                      <span className="text-caption leading-snug text-ink-400">{field.hint}</span>
                    </label>
                  );
                })}
              </div>
            </Card>
          ))}
      <Card className="p-5">
        <SectionHeading title="Logo artwork" />
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
          <span className="flex h-20 w-20 items-center justify-center overflow-hidden rounded-lg border border-rule bg-white" aria-hidden="true">
            {logoPreview ? (
              <img src={logoPreview} alt="" className="h-full w-full object-contain" />
            ) : (
              <span className="px-2 text-center text-caption text-ink-400">Built-in artwork</span>
            )}
          </span>
          <div className="grid flex-1 gap-3">
            <p className="max-w-xl text-body text-ink-600">
              {hasLogo
                ? "Custom artwork prints on new quotations and previews."
                : "No custom artwork: documents use the built-in logo."}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <label className="cursor-pointer text-caption font-medium text-accent-700 hover:underline">
                {logoBusy ? "Uploading…" : hasLogo ? "Replace artwork" : "Upload artwork"}
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  className="sr-only"
                  disabled={logoBusy}
                  onChange={(event) => {
                    void pickLogo(event.target.files?.[0]);
                    event.target.value = "";
                  }}
                />
              </label>
              {hasLogo ? (
                <button
                  type="button"
                  onClick={() => void removeLogo()}
                  disabled={logoBusy}
                  className="text-caption text-ink-400 hover:text-danger-600 hover:underline disabled:opacity-50"
                >
                  Remove
                </button>
              ) : null}
            </div>
          </div>
        </div>
        <p className="mt-3 text-caption leading-relaxed text-ink-400">
          PNG, JPEG, GIF, or WebP under 512KB. Past revisions keep whatever printed.
        </p>
      </Card>
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
            <dl className="grid gap-1.5">
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
                        <span className="block text-caption text-ink-400">{action.hint}</span>
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
                        className={`tnum rounded-md border px-2 py-1 font-mono text-caption transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
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
