import { useMemo, useState } from "react";
import { KeyRound, Lock, RotateCcw, ShieldCheck } from "lucide-react";
import { Modal } from "./Modal";
import { resetSecurityVault, setupSecurityVault, unlockSecurityVault } from "../lib/api";
import { trackClientTelemetry } from "../lib/telemetry";
import type { AuthStatus, SecurityVaultStatus } from "../lib/types";

type SecurityVaultModalProps = {
  authStatus: AuthStatus;
  onRefresh: () => Promise<void>;
  onLogin: () => Promise<void>;
};

const text = {
  title: "Security Vault",
  migrationTitle: "Choose how EVE Flipper stores your EVE auth locally",
  migrationBody:
    "Starting with v1.6.6, EVE auth tokens and selected private local database fields are stored through an encrypted vault. Existing sessions are intentionally logged out once so old plaintext auth can be purged.",
  standardTitle: "Standard Vault",
  standardBody:
    "Uses your operating system / local machine protection. Best default for most users. Updates keep working on the same device.",
  privateTitle: "Private Vault",
  privateBody:
    "Uses your passphrase and Argon2id. Stronger local privacy, but you must unlock it after app start. If the passphrase is lost, encrypted data cannot be recovered.",
  setupStandard: "Use Standard Vault",
  setupPrivate: "Create Private Vault",
  passphrase: "Vault passphrase",
  confirm: "Confirm passphrase",
  unlockTitle: "Unlock Private Vault",
  unlockBody: "Enter your vault passphrase to decrypt local EVE auth data for this session.",
  unlock: "Unlock",
  loginAgain: "Login with EVE again",
  continueWithoutLogin: "Continue without EVE login",
  resetTitle: "Lost passphrase?",
  resetBody: "Reset removes encrypted auth data for this local profile. This cannot decrypt the old data.",
  reset: "Reset encrypted profile",
  mismatch: "Passphrases do not match.",
  short: "Passphrase must be at least 8 characters.",
  done: "Vault is ready.",
  protectedFields: "Protected local fields",
  analyticsNote: "Market prices, dates and numeric aggregates remain queryable so charts and portfolio analytics keep working.",
};

function shouldShowVaultModal(status?: SecurityVaultStatus): boolean {
  if (!status?.available) return false;
  return Boolean(status.security_migration_required || status.private_unlock_required);
}

export function SecurityVaultModal({ authStatus, onRefresh, onLogin }: SecurityVaultModalProps) {
  const vault = authStatus.security_vault;
  const [passphrase, setPassphrase] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [resetConfirm, setResetConfirm] = useState("");
  const [showReset, setShowReset] = useState(false);
  const [setupDone, setSetupDone] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  const lockedPrivate = Boolean(vault?.private_unlock_required);
  const migrationRequired = Boolean(vault?.security_migration_required);
  const setupRequired = shouldShowVaultModal(vault);
  const setupDoneVisible =
    setupDone && Boolean(vault?.configured) && !authStatus.logged_in && !lockedPrivate && !migrationRequired;
  const open = setupRequired || (setupDoneVisible && !dismissed);
  const canCreatePrivate = passphrase.length >= 8 && passphrase === confirm;

  const modeLabel = useMemo(() => {
    if (!vault?.configured) return "not configured";
    if (vault.mode === "private") return vault.locked ? "private / locked" : "private / unlocked";
    if (vault.mode === "standard") return "standard";
    return vault.mode || "configured";
  }, [vault]);

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setMessage("");
    try {
      await action();
    } catch (err) {
      setMessage(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const setupStandard = () =>
    run(async () => {
      trackClientTelemetry({ event_type: "vault_setup_clicked", module: "security", properties: { mode: "standard" } });
      await setupSecurityVault("standard");
      setDismissed(false);
      setSetupDone(true);
      await onRefresh();
    });

  const setupPrivate = () =>
    run(async () => {
      trackClientTelemetry({ event_type: "vault_setup_clicked", module: "security", properties: { mode: "private" } });
      if (passphrase.length < 8) {
        setMessage(text.short);
        return;
      }
      if (passphrase !== confirm) {
        setMessage(text.mismatch);
        return;
      }
      await setupSecurityVault("private", passphrase);
      setPassphrase("");
      setConfirm("");
      setDismissed(false);
      setSetupDone(true);
      await onRefresh();
    });

  const unlockPrivate = () =>
    run(async () => {
      await unlockSecurityVault(passphrase);
      setPassphrase("");
      setSetupDone(false);
      setDismissed(true);
      await onRefresh();
    });

  const resetVault = () =>
    run(async () => {
      if (resetConfirm !== "RESET") {
        setMessage("Type RESET to confirm.");
        return;
      }
      await resetSecurityVault(true);
      setPassphrase("");
      setConfirm("");
      setResetConfirm("");
      setShowReset(false);
      setSetupDone(false);
      setDismissed(false);
      await onRefresh();
    });

  const loginAgain = async () => {
    setSetupDone(false);
    setDismissed(true);
    await onLogin();
  };

  const continueWithoutLogin = () => {
    if (setupRequired) return;
    setSetupDone(false);
    setDismissed(true);
  };

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={continueWithoutLogin}
      title={text.title}
      width="max-w-5xl"
      closeOnBackdrop={false}
      showClose={!setupRequired}
    >
      <div className="space-y-4 p-5 text-sm text-eve-text">
        <div className="border border-eve-border bg-eve-panel/70 p-4">
          <div className="flex items-start gap-3">
            <ShieldCheck className="mt-1 h-5 w-5 shrink-0 text-eve-accent" />
            <div>
              <h3 className="text-base font-semibold text-eve-accent">
                {lockedPrivate ? text.unlockTitle : text.migrationTitle}
              </h3>
              <p className="mt-2 max-w-4xl text-eve-dim">
                {lockedPrivate ? text.unlockBody : text.migrationBody}
              </p>
              <div className="mt-3 text-xs uppercase tracking-wider text-eve-dim">
                Vault: <span className="text-eve-text">{modeLabel}</span>
              </div>
              {vault?.protected_fields?.length ? (
                <div className="mt-3 text-xs text-eve-dim">
                  <div className="uppercase tracking-wider">
                    {text.protectedFields}:{" "}
                    <span className="text-eve-text">{vault.protected_fields.length}</span>
                  </div>
                  <div className="mt-1">{text.analyticsNote}</div>
                </div>
              ) : null}
            </div>
          </div>
        </div>

        {setupDoneVisible ? (
          <div className="border border-green-500/40 bg-green-950/20 p-4">
            <h4 className="font-semibold text-green-300">{text.done}</h4>
            <p className="mt-2 text-eve-dim">
              Existing auth sessions were purged. You can continue using public market tools now, or login with EVE
              again to store a fresh encrypted session.
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={continueWithoutLogin}
                className="inline-flex items-center gap-2 bg-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-black disabled:opacity-50"
              >
                {text.continueWithoutLogin}
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={loginAgain}
                className="inline-flex items-center gap-2 border border-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-eve-accent disabled:opacity-50"
              >
                {text.loginAgain}
              </button>
            </div>
          </div>
        ) : lockedPrivate ? (
          <div className="grid gap-4 lg:grid-cols-[1fr_360px]">
            <div className="border border-eve-border bg-eve-panel/50 p-4">
              <label className="block text-xs uppercase tracking-widest text-eve-dim">{text.passphrase}</label>
              <input
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                className="mt-2 w-full border border-eve-border bg-eve-bg px-3 py-2 text-eve-text outline-none focus:border-eve-accent"
                autoFocus
              />
              <button
                type="button"
                disabled={busy || passphrase.length === 0}
                onClick={unlockPrivate}
                className="mt-4 inline-flex items-center gap-2 bg-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-black disabled:opacity-50"
              >
                <Lock className="h-4 w-4" />
                {busy ? "..." : text.unlock}
              </button>
            </div>

            <div className="border border-red-500/40 bg-red-950/20 p-4">
              <h4 className="font-semibold text-red-300">{text.resetTitle}</h4>
              <p className="mt-2 text-eve-dim">{text.resetBody}</p>
              {!showReset ? (
                <button
                  type="button"
                  onClick={() => setShowReset(true)}
                  className="mt-4 border border-red-500/50 px-3 py-2 text-red-200"
                >
                  {text.reset}
                </button>
              ) : (
                <div className="mt-4 space-y-2">
                  <input
                    value={resetConfirm}
                    onChange={(e) => setResetConfirm(e.target.value)}
                    placeholder="RESET"
                    className="w-full border border-red-500/40 bg-eve-bg px-3 py-2 text-eve-text outline-none"
                  />
                  <button
                    type="button"
                    disabled={busy || resetConfirm !== "RESET"}
                    onClick={resetVault}
                    className="inline-flex items-center gap-2 border border-red-500/60 px-3 py-2 text-red-200 disabled:opacity-50"
                  >
                    <RotateCcw className="h-4 w-4" />
                    Confirm reset
                  </button>
                </div>
              )}
            </div>
          </div>
        ) : (
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="border border-eve-accent/40 bg-eve-accent/5 p-4">
              <div className="flex items-center gap-2 text-eve-accent">
                <ShieldCheck className="h-5 w-5" />
                <h4 className="font-semibold uppercase tracking-wider">{text.standardTitle}</h4>
              </div>
              <p className="mt-3 min-h-[72px] text-eve-dim">{text.standardBody}</p>
              <button
                type="button"
                disabled={busy}
                onClick={setupStandard}
                className="mt-4 bg-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-black disabled:opacity-50"
              >
                {busy ? "..." : text.setupStandard}
              </button>
            </div>

            <div className="border border-eve-border bg-eve-panel/50 p-4">
              <div className="flex items-center gap-2 text-eve-accent">
                <KeyRound className="h-5 w-5" />
                <h4 className="font-semibold uppercase tracking-wider">{text.privateTitle}</h4>
              </div>
              <p className="mt-3 text-eve-dim">{text.privateBody}</p>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <label className="block">
                  <span className="text-xs uppercase tracking-widest text-eve-dim">{text.passphrase}</span>
                  <input
                    type="password"
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                    className="mt-2 w-full border border-eve-border bg-eve-bg px-3 py-2 text-eve-text outline-none focus:border-eve-accent"
                  />
                </label>
                <label className="block">
                  <span className="text-xs uppercase tracking-widest text-eve-dim">{text.confirm}</span>
                  <input
                    type="password"
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    className="mt-2 w-full border border-eve-border bg-eve-bg px-3 py-2 text-eve-text outline-none focus:border-eve-accent"
                  />
                </label>
              </div>
              <button
                type="button"
                disabled={busy || !canCreatePrivate}
                onClick={setupPrivate}
                className="mt-4 border border-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-eve-accent disabled:opacity-50"
              >
                {busy ? "..." : text.setupPrivate}
              </button>
            </div>
          </div>
        )}

        {migrationRequired && !lockedPrivate && !setupDoneVisible && (
          <div className="border border-yellow-500/40 bg-yellow-950/20 px-4 py-3 text-yellow-200">
            After setup, login with EVE again. Old sessions are purged intentionally.
          </div>
        )}

        {vault?.configured && !lockedPrivate && !setupDoneVisible && (
          <button
            type="button"
            disabled={busy}
            onClick={loginAgain}
            className="inline-flex items-center gap-2 border border-eve-accent px-4 py-2 font-semibold uppercase tracking-wider text-eve-accent disabled:opacity-50"
          >
            {text.loginAgain}
          </button>
        )}

        {message && <div className="border border-eve-border bg-eve-bg px-4 py-3 text-eve-dim">{message}</div>}
      </div>
    </Modal>
  );
}
