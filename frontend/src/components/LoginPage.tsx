import { useState } from "react";
import { Login } from "@backend/backend/AuthAPI";
import { Button, Card, StatusBanner } from "./ui";

export interface SignedInUser {
  ID: number;
  Email: string;
  DisplayName: string;
  Role: string;
}

export function LoginPage({ onSignedIn }: { onSignedIn: (user: SignedInUser) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const user = (await Login(email.trim(), password)) as SignedInUser;
      onSignedIn(user);
    } catch (err: unknown) {
      setError(typeof err === "string" ? err : "We couldn't sign you in. Try again.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-full items-center justify-center px-5 py-16">
      <div className="w-full max-w-[400px]">
        <div className="mb-6 text-center">
          <p className="text-display font-semibold tracking-[-0.03em] text-ink-950">Vera</p>
          <p className="mt-1.5 text-body text-ink-500">
            Quotation Generator for PT Arcon Penta Persada
          </p>
          <p className="mt-3 inline-block rounded-md border border-rule bg-white px-2.5 py-1 text-micro font-medium uppercase tracking-[0.14em] text-ink-500">
            Sign in to continue
          </p>
        </div>
        <Card className="p-6">
          <form onSubmit={submit} className="flex flex-col gap-4">
            {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Email</span>
              <input
                type="email"
                autoComplete="username"
                autoFocus
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@company.com"
                className="w-full rounded-md border border-rule bg-white px-3 py-2.5 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Password</span>
              <input
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="••••••••"
                className="w-full rounded-md border border-rule bg-white px-3 py-2.5 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
              />
            </label>
            <Button type="submit" variant="primary" disabled={busy} className="mt-1 w-full">
              {busy ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </Card>
        <p className="mt-4 text-center text-caption leading-relaxed text-ink-400">
          Accounts are created by your administrator.
          <br />
          Contact them if you cannot sign in.
        </p>
      </div>
    </div>
  );
}
