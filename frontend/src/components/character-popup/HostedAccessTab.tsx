import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Check, Clock3, Copy, RefreshCw, X } from "lucide-react";
import type { HostedAccessPlanOffer, HostedAccessStatus } from "../../lib/types";
import { trackClientTelemetry } from "../../lib/telemetry";

interface HostedAccessTabProps {
  access: HostedAccessStatus | null;
  loading: boolean;
  error: string | null;
  lastCheckedAt: Date | null;
  onReload: () => void;
  onRequestPayment: (planId: string) => Promise<void>;
  onCancelPayment: () => Promise<void>;
  formatIsk: (value: number) => string;
}

type HostedPaymentHistoryRow = NonNullable<HostedAccessStatus["payment_history"]>[number];
type HostedAccessPayment = NonNullable<HostedAccessStatus["payment"]>;

function formatDate(value?: string | Date | null) {
  if (!value) return "N/A";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleDateString() + " " + date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function statusTone(status?: string) {
  const normalized = (status ?? "").toLowerCase();
  if (normalized === "active" || normalized === "trial" || normalized === "grace") return "text-eve-success border-eve-success/40 bg-eve-success/10";
  if (normalized === "expired" || normalized === "blocked") return "text-eve-error border-eve-error/40 bg-eve-error/10";
  return "text-eve-accent border-eve-accent/40 bg-eve-accent/10";
}

function paymentStatusTone(status?: string) {
  const normalized = (status ?? "").toLowerCase();
  if (normalized === "matched") return "text-eve-success border-eve-success/40 bg-eve-success/10";
  if (
    normalized === "underpaid" ||
    normalized === "expired" ||
    normalized === "unmatched" ||
    normalized === "sender_mismatch" ||
    normalized === "receiver_mismatch"
  ) {
    return "text-eve-error border-eve-error/40 bg-eve-error/10";
  }
  if (normalized === "duplicate" || normalized === "cancelled") return "text-eve-dim border-eve-border bg-eve-dark/60";
  return "text-eve-accent border-eve-accent/40 bg-eve-accent/10";
}

function receiverDisplay(payment: HostedAccessPayment) {
  const name = payment.receiver_name || "Configured receiver";
  if (payment.receiver_character_id) return `${name} (character ${payment.receiver_character_id})`;
  if (payment.receiver_corporation_id) return `${name} (corporation ${payment.receiver_corporation_id})`;
  return name;
}

function receiverInstructionLines(payment: HostedAccessPayment) {
  const lines = [`Receiver character name: ${payment.receiver_name || "Configured receiver"}`];
  if (payment.receiver_character_id) lines.push(`Receiver character ID: ${payment.receiver_character_id}`);
  if (payment.receiver_corporation_id) lines.push(`Receiver corporation ID: ${payment.receiver_corporation_id}`);
  return lines;
}

function formatStatus(value?: string) {
  return (value || "unknown").replace(/_/g, " ");
}

function planLimitSummary(plan: HostedAccessPlanOffer) {
  const parts = [`${plan.period_days} days`];
  parts.push(plan.scan_limit_per_day != null ? `${plan.scan_limit_per_day} scans/day` : "unlimited scans");
  if (plan.features?.includes("station_ai") || plan.station_ai_limit_per_day != null) {
    parts.push(plan.station_ai_limit_per_day != null ? `${plan.station_ai_limit_per_day} AI/day` : "unlimited AI");
  }
  return parts.join(" - ");
}

function exactIskAmount(value: number) {
  if (!Number.isFinite(value)) return "0";
  return String(Math.trunc(value));
}

function formatCountdown(expiresAt?: string, now = Date.now()) {
  if (!expiresAt) return "No expiry time";
  const expires = new Date(expiresAt).getTime();
  if (Number.isNaN(expires)) return "Unknown expiry";
  const remainingMs = expires - now;
  if (remainingMs <= 0) return "Expired";
  const totalSeconds = Math.ceil(remainingMs / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h left`;
  if (hours > 0) return `${hours}h ${minutes}m left`;
  return `${minutes}m left`;
}

function paymentDelta(row: HostedPaymentHistoryRow) {
  if (!row.matched_amount_isk) return null;
  return row.matched_amount_isk - row.amount_isk;
}

function latestPaymentStatus(history: HostedAccessStatus["payment_history"]) {
  return history && history.length > 0 ? history[0]?.status?.toLowerCase() ?? "" : "";
}

function paymentHistoryHint(status?: string) {
  switch ((status ?? "").toLowerCase()) {
    case "pending":
      return "Waiting for wallet journal polling.";
    case "matched":
      return "Activated automatically.";
    case "underpaid":
      return "Amount is lower than required. Owner review is needed.";
    case "overpaid":
      return "Access activated; extra credit is manual-only.";
    case "expired":
      return "Transfer was after request expiry. Create a fresh request.";
    case "duplicate":
      return "This wallet reference was already processed.";
    case "cancelled":
      return "Cancelled before ISK was sent.";
    case "sender_mismatch":
      return "Sender does not match the character that created the request.";
    case "receiver_mismatch":
      return "Transfer went to a different receiver wallet.";
    case "unmatched":
      return "No active request matched the wallet row by code, sender, and exact amount.";
    default:
      return "";
  }
}

function paymentState(access: HostedAccessStatus | null, paymentHistory: HostedAccessStatus["payment_history"]) {
  const status = (access?.status ?? "").toLowerCase();
  const latest = latestPaymentStatus(paymentHistory);
  if (access?.payment) {
    return {
      tone: "text-eve-accent border-eve-accent/40 bg-eve-accent/10",
      title: "Waiting for payment",
      body: "Send the exact ISK amount to the receiver. If EVE shows a Reason / Description field, paste the optional code too. Wallet polling activates access after the transfer is visible.",
    };
  }
  if (status === "active" || status === "trial" || status === "grace") {
    return {
      tone: "text-eve-success border-eve-success/40 bg-eve-success/10",
      title: status === "grace" ? "Subscription in grace period" : "Subscription active",
      body: "Paid access is enabled. Usage counters below show the current billing window.",
    };
  }
  if (latest === "matched") {
    return {
      tone: "text-eve-success border-eve-success/40 bg-eve-success/10",
      title: "Payment found",
      body: "The payment was matched. Refresh status if the subscription state has not updated yet.",
    };
  }
  if (latest === "underpaid" || latest === "expired" || latest === "unmatched" || latest === "sender_mismatch" || latest === "receiver_mismatch") {
    return {
      tone: "text-eve-error border-eve-error/40 bg-eve-error/10",
      title: "Payment needs review",
      body: paymentHistoryHint(latest) || "The latest payment attempt could not be activated automatically. Check payment history or contact support.",
    };
  }
  return {
    tone: "text-eve-dim border-eve-border bg-eve-dark/55",
    title: "Choose a plan",
    body: "Create a payment request, send the exact ISK amount to the receiver, then refresh status.",
  };
}

export function HostedAccessTab({ access, loading, error, lastCheckedAt, onReload, onRequestPayment, onCancelPayment, formatIsk }: HostedAccessTabProps) {
  const [requestingPlan, setRequestingPlan] = useState<string | null>(null);
  const [cancelingPayment, setCancelingPayment] = useState(false);
  const [paymentError, setPaymentError] = useState<string | null>(null);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [showPlanPicker, setShowPlanPicker] = useState(false);
  const [selectedPlanId, setSelectedPlanId] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const usageEntries = Object.entries(access?.usage ?? {});
  const featureEntries = Object.entries(access?.features ?? {}).filter(([, enabled]) => enabled);
  const planOffers = access?.available_plans ?? [];
  const paymentHistory = access?.payment_history ?? [];
  const state = paymentState(access, paymentHistory);
  const refreshLabel = loading ? "Checking..." : "Refresh";
  const refreshStatusLabel = loading ? "Checking..." : "Refresh status";
  const payment = access?.payment;
  const pendingHistoryRow = useMemo(
    () => paymentHistory.find((row) => row.code === payment?.reason_code || row.status?.toLowerCase() === "pending"),
    [payment?.reason_code, paymentHistory]
  );
  const pendingPlan = planOffers.find((plan) => plan.id === pendingHistoryRow?.plan_id);
  const pendingCountdown = formatCountdown(payment?.expires_at, now);
  const paymentExpired = pendingCountdown === "Expired";

  useEffect(() => {
    if (!payment?.expires_at) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [payment?.expires_at]);

  useEffect(() => {
    if (!payment) {
      setShowPlanPicker(false);
    }
    setSelectedPlanId(null);
  }, [payment]);

  const copyText = async (key: string, text: string, copyMode: string) => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopiedKey(key);
      window.setTimeout(() => setCopiedKey((current) => (current === key ? null : current)), 1800);
      if (access?.plan) {
        trackClientTelemetry({
          event_type: "payment_instructions_copied",
          module: "hosted",
          properties: {
            plan: access.plan.id,
            amount_isk: payment?.amount_isk,
            copy_mode: copyMode,
          },
        });
      }
    } catch {
      // Clipboard support is best-effort in desktop/web shells.
    }
  };

  const copyPaymentCode = async () => {
    if (!payment?.reason_code) return;
    await copyText("reason", payment.reason_code, "reason_code");
  };

  const copyPaymentInstructions = async () => {
    if (!payment?.reason_code) return;
    const lines = [
      "EVE Flipper hosted access payment",
      pendingPlan ? `Plan: ${pendingPlan.name} (${pendingPlan.id})` : pendingHistoryRow?.plan_id ? `Plan: ${pendingHistoryRow.plan_id}` : "",
      ...receiverInstructionLines(payment),
      `Exact amount: ${exactIskAmount(payment.amount_isk)} ISK`,
      `Optional Reason / Description code: ${payment.reason_code}`,
      `Request expires: ${formatDate(payment.expires_at)} (${formatCountdown(payment.expires_at, now)})`,
      "If your EVE transfer window has no Reason / Description field, leave it empty. The payment can still match by sender and exact amount.",
      "After sending: keep this tab open or press Refresh status. Wallet journal polling checks about every 20 seconds.",
    ].filter(Boolean);
    await copyText("all", lines.join("\n"), "full");
  };

  const createPaymentRequest = async (planId: string) => {
    if (!access) return;
    setRequestingPlan(planId);
    setPaymentError(null);
    const offer = planOffers.find((plan) => plan.id === planId);
    trackClientTelemetry({
      event_type: "plan_selected",
      module: "hosted",
      properties: {
        plan: planId,
        amount_isk: offer?.price_isk,
        subscription_status: access.status,
      },
    });
    try {
      await onRequestPayment(planId);
      setShowPlanPicker(false);
      setSelectedPlanId(null);
    } catch (e: any) {
      setPaymentError(e?.message || "Payment request failed");
    } finally {
      setRequestingPlan(null);
    }
  };

  const cancelPaymentRequest = async () => {
    if (!payment || cancelingPayment) return;
    const confirmed = window.confirm("Cancel the current pending payment request? If you already sent ISK, wait for the wallet match instead.");
    if (!confirmed) return;
    setCancelingPayment(true);
    setPaymentError(null);
    trackClientTelemetry({
      event_type: "payment_request_cancelled",
      module: "hosted",
      properties: {
        plan: pendingPlan?.id ?? pendingHistoryRow?.plan_id,
        amount_isk: payment.amount_isk,
      },
    });
    try {
      await onCancelPayment();
      setShowPlanPicker(false);
      setSelectedPlanId(null);
    } catch (e: any) {
      setPaymentError(e?.message || "Payment cancel failed");
    } finally {
      setCancelingPayment(false);
    }
  };

  const renderCopyButton = (key: string, label: string, value: string, copyMode: string) => (
    <button
      type="button"
      onClick={() => { void copyText(key, value, copyMode); }}
      className="inline-flex items-center justify-center gap-1.5 border border-eve-border bg-eve-dark/75 px-2.5 py-1 text-[10px] uppercase tracking-[0.12em] text-eve-dim hover:border-eve-accent/60 hover:text-eve-accent"
    >
      {copiedKey === key ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      {copiedKey === key ? "Copied" : label}
    </button>
  );

  const renderPlanPicker = (title = "Choose a plan", intro = "Pick a tariff to generate a fresh ISK payment request.") => (
    <div className="space-y-3">
      <div>
        <div className="text-[10px] uppercase tracking-[0.18em] text-eve-dim">{title}</div>
        <div className="mt-1 text-xs text-eve-dim">{intro}</div>
      </div>
      {payment && (
        <div className="border border-eve-accent/35 bg-eve-accent/10 px-3 py-2 text-xs text-eve-accent">
          No payment request is created when you click a plan. Select a plan first, then confirm with the button below.
        </div>
      )}
      {planOffers.length === 0 ? (
        <div className="text-eve-dim">No plans are available right now.</div>
      ) : (
        <>
          <div className="grid gap-2">
            {planOffers.map((plan) => {
              const selected = plan.id === selectedPlanId;
              const hasPendingRequest = plan.id === pendingPlan?.id;
              return (
                <button
                  key={plan.id}
                  type="button"
                  onClick={() => {
                    setSelectedPlanId(plan.id);
                    setPaymentError(null);
                  }}
                  disabled={requestingPlan !== null}
                  className={`w-full border px-3 py-2 text-left transition-colors disabled:opacity-50 ${
                    selected
                      ? "border-eve-accent bg-eve-accent/15"
                      : "border-eve-border bg-eve-dark/65 hover:border-eve-accent/60 hover:bg-eve-accent/10"
                  }`}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-semibold text-eve-text">{plan.name}</span>
                    <span className="font-mono text-eve-accent">{formatIsk(plan.price_isk)} ISK</span>
                  </div>
                  <div className="mt-1 text-xs text-eve-dim">{planLimitSummary(plan)}</div>
                  <div className="mt-1 flex flex-wrap gap-2 text-[10px] uppercase tracking-[0.12em]">
                    {selected && <span className="text-eve-accent">Selected, not pending yet</span>}
                    {hasPendingRequest && <span className="text-eve-dim">Current pending request</span>}
                  </div>
                </button>
              );
            })}
          </div>
          <button
            type="button"
            onClick={() => {
              if (selectedPlanId) void createPaymentRequest(selectedPlanId);
            }}
            disabled={!selectedPlanId || requestingPlan !== null}
            className="w-full bg-eve-accent px-3 py-2 font-semibold uppercase tracking-[0.12em] text-black hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-45"
          >
            {requestingPlan
              ? "Creating payment request..."
              : selectedPlanId
                ? `Create payment request for ${planOffers.find((plan) => plan.id === selectedPlanId)?.name ?? selectedPlanId}`
                : "Select a plan first"}
          </button>
        </>
      )}
      {paymentError && <div className="border border-eve-error/40 bg-eve-error/10 px-3 py-2 text-xs text-eve-error">{paymentError}</div>}
    </div>
  );

  if (loading && !access) {
    return <div className="flex h-full items-center justify-center text-eve-dim">Loading access...</div>;
  }

  return (
    <div className="space-y-4 text-sm">
      <div className="grid gap-3 md:grid-cols-[1.2fr_0.8fr]">
        <section className="border border-eve-border bg-eve-panel/65 p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="text-[10px] uppercase tracking-[0.2em] text-eve-dim">Current access</div>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <span className="text-2xl font-semibold text-eve-text">{access?.plan.name ?? "Unknown"}</span>
                <span className={`rounded-sm border px-2 py-0.5 text-[10px] uppercase tracking-[0.18em] ${statusTone(access?.status)}`}>
                  {access?.status ?? "unknown"}
                </span>
                {!access?.hosted && (
                  <span className="rounded-sm border border-eve-border bg-eve-dark px-2 py-0.5 text-[10px] uppercase tracking-[0.18em] text-eve-dim">
                    local
                  </span>
                )}
              </div>
            </div>
            <button
              type="button"
              onClick={onReload}
              disabled={loading}
              className="border border-eve-border bg-eve-dark px-3 py-1.5 text-[11px] uppercase tracking-[0.14em] text-eve-dim hover:border-eve-accent/60 hover:text-eve-accent disabled:opacity-50"
            >
              {refreshLabel}
            </button>
          </div>

          {error && <div className="mt-3 border border-eve-error/40 bg-eve-error/10 px-3 py-2 text-eve-error">{error}</div>}
          {access?.message && <div className="mt-3 text-eve-dim">{access.message}</div>}
          {lastCheckedAt && <div className="mt-3 text-xs text-eve-dim">Last checked {formatDate(lastCheckedAt)}</div>}
          <div className={`mt-4 border px-3 py-2 ${state.tone}`}>
            <div className="text-xs font-semibold uppercase tracking-[0.16em]">{state.title}</div>
            <div className="mt-1 text-xs opacity-85">{state.body}</div>
          </div>

          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <div className="border border-eve-border/70 bg-eve-dark/45 p-3">
              <div className="text-[10px] uppercase tracking-[0.18em] text-eve-dim">Expires</div>
              <div className="mt-1 text-eve-text">{formatDate(access?.plan.expires_at)}</div>
            </div>
            <div className="border border-eve-border/70 bg-eve-dark/45 p-3">
              <div className="text-[10px] uppercase tracking-[0.18em] text-eve-dim">Renews</div>
              <div className="mt-1 text-eve-text">{formatDate(access?.plan.renews_at)}</div>
            </div>
          </div>
        </section>

        <section className="border border-eve-border bg-eve-panel/65 p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="text-[10px] uppercase tracking-[0.2em] text-eve-dim">Payment</div>
            <button
              type="button"
              onClick={onReload}
              disabled={loading}
              className="border border-eve-border bg-eve-dark px-2.5 py-1 text-[10px] uppercase tracking-[0.14em] text-eve-dim hover:border-eve-accent/60 hover:text-eve-accent disabled:opacity-50"
            >
              {refreshStatusLabel}
            </button>
          </div>
          {payment && !showPlanPicker ? (
            <div className="mt-3 space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2 border border-eve-accent/35 bg-eve-accent/10 px-3 py-2 text-xs text-eve-accent">
                <div className="flex min-w-0 items-center gap-2">
                  <Clock3 className="h-4 w-4 shrink-0" />
                  <span>
                    Pending request{pendingPlan ? ` for ${pendingPlan.name}` : ""}. Wallet polling checks about every 20 seconds.
                  </span>
                </div>
                <span className={paymentExpired ? "font-semibold text-eve-error" : "font-semibold text-eve-accent"}>{pendingCountdown}</span>
              </div>

              <div className="border border-eve-border/70 bg-eve-dark/40 p-3">
                <div className="grid gap-2 lg:grid-cols-[1.1fr_0.75fr_1fr]">
                  <div className="min-w-0 border border-eve-border/60 bg-eve-panel/45 p-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="text-[10px] uppercase tracking-[0.14em] text-eve-dim">1. Receiver</div>
                        <div className="mt-1 break-words text-eve-text">{receiverDisplay(payment)}</div>
                      </div>
                      {renderCopyButton("receiver", "Copy", payment.receiver_name || receiverDisplay(payment), "receiver")}
                    </div>
                  </div>
                  <div className="min-w-0 border border-eve-border/60 bg-eve-panel/45 p-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="text-[10px] uppercase tracking-[0.14em] text-eve-dim">2. Exact ISK</div>
                        <div className="mt-1 font-mono text-eve-accent">{exactIskAmount(payment.amount_isk)}</div>
                        <div className="mt-0.5 text-[11px] text-eve-dim">{formatIsk(payment.amount_isk)} ISK</div>
                      </div>
                      {renderCopyButton("amount", "Copy", exactIskAmount(payment.amount_isk), "amount")}
                    </div>
                  </div>
                  <div className="min-w-0 border border-eve-border/60 bg-eve-panel/45 p-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="text-[10px] uppercase tracking-[0.14em] text-eve-dim">3. Optional code</div>
                        <button
                          type="button"
                          onClick={copyPaymentCode}
                          className="mt-1 break-all text-left font-mono text-eve-accent hover:text-eve-accent/80"
                        >
                          {payment.reason_code}
                        </button>
                      </div>
                      {renderCopyButton("reason", "Copy", payment.reason_code, "reason_code")}
                    </div>
                  </div>
                </div>
                <div className="mt-2 border border-eve-accent/25 bg-eve-accent/10 px-2 py-2 text-xs leading-relaxed text-eve-dim">
                  In EVE: Wallet {"->"} Send ISK, choose the receiver and send the exact amount. If the transfer window has
                  a Reason / Description field, paste the optional code. If there is no such field, leave it empty.
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={copyPaymentInstructions}
                  className="inline-flex items-center gap-1.5 border border-eve-border bg-eve-dark/70 px-3 py-2 text-xs uppercase tracking-[0.12em] text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
                >
                  {copiedKey === "all" ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                  {copiedKey === "all" ? "Copied instructions" : "Copy full instructions"}
                </button>
                <button
                  type="button"
                  onClick={onReload}
                  disabled={loading}
                  className="inline-flex items-center gap-1.5 border border-eve-border bg-eve-dark/70 px-3 py-2 text-xs uppercase tracking-[0.12em] text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
                >
                  <RefreshCw className="h-3.5 w-3.5" />
                  {refreshStatusLabel}
                </button>
                <button
                  type="button"
                  onClick={() => setShowPlanPicker(true)}
                  className="inline-flex items-center gap-1.5 border border-eve-border bg-eve-dark/70 px-3 py-2 text-xs uppercase tracking-[0.12em] text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent disabled:opacity-50"
                >
                  <ArrowLeft className="h-3.5 w-3.5" />
                  Choose another plan
                </button>
                <button
                  type="button"
                  onClick={() => { void cancelPaymentRequest(); }}
                  disabled={cancelingPayment}
                  className="inline-flex items-center gap-1.5 border border-eve-error/45 bg-eve-error/10 px-3 py-2 text-xs uppercase tracking-[0.12em] text-eve-error hover:bg-eve-error/15 disabled:opacity-50"
                >
                  <X className="h-3.5 w-3.5" />
                  {cancelingPayment ? "Canceling..." : "Cancel pending"}
                </button>
              </div>
              {paymentError && <div className="border border-eve-error/40 bg-eve-error/10 px-3 py-2 text-xs text-eve-error">{paymentError}</div>}
              <div className="text-xs leading-relaxed text-eve-dim">
                Valid until {formatDate(payment.expires_at)}. After sending ISK, wallet journal visibility usually takes 1-3 minutes.
              </div>
            </div>
          ) : payment && showPlanPicker ? (
            <div className="mt-3 space-y-3">
              <button
                type="button"
                onClick={() => setShowPlanPicker(false)}
                className="inline-flex items-center gap-1.5 border border-eve-border bg-eve-dark/70 px-3 py-1.5 text-xs uppercase tracking-[0.12em] text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Back to current payment
              </button>
              {renderPlanPicker("Choose another plan", "Select a tariff first. A new pending request is created only after you press the confirmation button. Use only the latest request when sending ISK.")}
            </div>
          ) : (access?.status === "active" || access?.status === "trial" || access?.status === "grace") ? (
            <div className="mt-3 space-y-3">
              <div className="border border-eve-success/35 bg-eve-success/10 px-3 py-2 text-xs text-eve-success">
                No pending payment. Current hosted access is active.
              </div>
              {renderPlanPicker("Extend or change plan", "Select a tariff, then press the confirmation button to create a payment request.")}
            </div>
          ) : planOffers.length > 0 ? (
            <div className="mt-3">{renderPlanPicker("Choose a plan", "Select a tariff, then press the confirmation button to create a payment request.")}</div>
          ) : (
            <div className="mt-3 text-eve-dim">No pending payment request.</div>
          )}
        </section>
      </div>

      <section className="border border-eve-border bg-eve-panel/65 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[10px] uppercase tracking-[0.2em] text-eve-dim">Payment history</div>
          {paymentHistory.length > 0 && <div className="text-xs text-eve-dim">latest {paymentHistory.length}</div>}
        </div>
        {paymentHistory.length === 0 ? (
          <div className="mt-3 text-eve-dim">No payment attempts yet.</div>
        ) : (
          <div className="mt-3 overflow-x-auto">
            <table className="min-w-full text-left text-xs">
              <thead className="border-b border-eve-border text-[10px] uppercase tracking-[0.16em] text-eve-dim">
                <tr>
                  <th className="py-2 pr-3 font-medium">Status</th>
                  <th className="py-2 pr-3 font-medium">Plan</th>
                  <th className="py-2 pr-3 font-medium">Required</th>
                  <th className="py-2 pr-3 font-medium">Paid</th>
                  <th className="py-2 pr-3 font-medium">Code</th>
                  <th className="py-2 pr-3 font-medium">Created</th>
                  <th className="py-2 pr-3 font-medium">Matched</th>
                  <th className="py-2 pr-3 font-medium">Note</th>
                </tr>
              </thead>
              <tbody>
                {paymentHistory.map((row) => {
                  const delta = paymentDelta(row);
                  return (
                    <tr key={`${row.code}-${row.created_at}`} className="border-b border-eve-border/55">
                      <td className="py-2 pr-3">
                        <span className={`inline-flex rounded-sm border px-2 py-0.5 uppercase tracking-[0.12em] ${paymentStatusTone(row.status)}`}>
                          {formatStatus(row.status)}
                        </span>
                      </td>
                      <td className="py-2 pr-3 font-semibold text-eve-text">{row.plan_id}</td>
                      <td className="py-2 pr-3 font-mono text-eve-accent">{formatIsk(row.amount_isk)} ISK</td>
                      <td className="py-2 pr-3 font-mono">
                        {row.matched_amount_isk ? (
                          <div className="space-y-0.5">
                            <div className="text-eve-text">{formatIsk(row.matched_amount_isk)} ISK</div>
                            {delta !== 0 && (
                              <div className={delta && delta > 0 ? "text-eve-success" : "text-eve-error"}>
                                {delta && delta > 0 ? "+" : ""}{formatIsk(delta ?? 0)} ISK
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className="text-eve-dim">-</span>
                        )}
                      </td>
                      <td className="py-2 pr-3 font-mono text-eve-text">{row.code}</td>
                      <td className="py-2 pr-3 text-eve-dim">{formatDate(row.created_at)}</td>
                      <td className="py-2 pr-3 text-eve-dim">{row.matched_at ? formatDate(row.matched_at) : "-"}</td>
                      <td className="py-2 pr-3 text-eve-dim">{row.note || paymentHistoryHint(row.status) || "-"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <div className="grid gap-3 lg:grid-cols-[1fr_1fr]">
        <section className="border border-eve-border bg-eve-panel/65 p-4">
          <div className="text-[10px] uppercase tracking-[0.2em] text-eve-dim">Usage</div>
          <div className="mt-3 space-y-3">
            {usageEntries.length === 0 && <div className="text-eve-dim">No usage counters.</div>}
            {usageEntries.map(([key, usage]) => {
              const limit = usage.limit ?? null;
              const remaining = usage.remaining ?? null;
              const pct = limit && limit > 0 ? Math.min(100, Math.max(0, (usage.used / limit) * 100)) : 0;
              return (
                <div key={key} className="border border-eve-border/70 bg-eve-dark/45 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <span className="uppercase tracking-[0.16em] text-eve-dim">{key}</span>
                    <span className="text-eve-text">
                      {usage.used} / {limit ?? "unlimited"}
                    </span>
                  </div>
                  <div className="mt-2 h-1.5 bg-eve-dark">
                    <div className="h-full bg-eve-accent" style={{ width: `${pct}%` }} />
                  </div>
                  <div className="mt-2 flex justify-between text-xs text-eve-dim">
                    <span>{usage.window}</span>
                    <span>{remaining == null ? "unlimited" : `${remaining} left`}</span>
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        <section className="border border-eve-border bg-eve-panel/65 p-4">
          <div className="text-[10px] uppercase tracking-[0.2em] text-eve-dim">Enabled features</div>
          <div className="mt-3 flex flex-wrap gap-2">
            {featureEntries.length === 0 && <div className="text-eve-dim">No feature flags.</div>}
            {featureEntries.map(([feature]) => (
              <span key={feature} className="border border-eve-border bg-eve-dark px-2 py-1 text-xs text-eve-text">
                {feature.replace(/_/g, " ")}
              </span>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
