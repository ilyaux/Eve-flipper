/* @vitest-environment jsdom */

import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { formatHostedPaymentCountdown, hostedPaymentState } from "./HostedAccessTab";
import { HostedAccessTab } from "./HostedAccessTab";
import type { HostedAccessStatus } from "../../lib/types";

const baseAccess: HostedAccessStatus = {
  hosted: true,
  plan: { id: "free", name: "Free" },
  status: "free",
  features: {},
  usage: {},
};

describe("hosted access payment state", () => {
  it("prioritizes active access over a pending extension request", () => {
    const state = hostedPaymentState(
      {
        ...baseAccess,
        status: "active",
        plan: { id: "trader", name: "Trader" },
        payment: {
          receiver_name: "EVE Flipper Billing",
          receiver_character_id: 2124476406,
          amount_isk: 300_000_000,
          reason_code: "EFLIP-TEST",
        },
      },
      [{ code: "EFLIP-TEST", plan_id: "trader", amount_isk: 300_000_000, status: "pending", created_at: "2026-06-19T00:00:00Z", expires_at: "2026-06-20T00:00:00Z" }],
    );

    expect(state.title).toBe("Subscription active");
    expect(state.tone).toContain("text-eve-success");
    expect(state.body).toContain("Paid access is already enabled");
  });

  it("explains pending ESI wallet journal visibility", () => {
    const state = hostedPaymentState(
      {
        ...baseAccess,
        payment: {
          receiver_name: "EVE Flipper Billing",
          receiver_character_id: 2124476406,
          amount_isk: 300_000_000,
          reason_code: "EFLIP-TEST",
        },
      },
      [],
    );

    expect(state.title).toBe("Waiting for payment");
    expect(state.body).toContain("CCP");
    expect(state.body).toContain("60 minutes");
  });

  it("shows manual review state for failed automatic matches", () => {
    const state = hostedPaymentState(baseAccess, [
      {
        code: "EFLIP-TEST",
        plan_id: "trader",
        amount_isk: 300_000_000,
        status: "sender_mismatch",
        created_at: "2026-06-19T00:00:00Z",
        expires_at: "2026-06-20T00:00:00Z",
      },
    ]);

    expect(state.title).toBe("Payment needs review");
    expect(state.tone).toContain("text-eve-error");
    expect(state.body).toContain("Sender");
  });
});

describe("HostedAccessTab billing UI", () => {
  const noopAsync = async () => {};

  it("does not create a pending payment until the selected plan is confirmed", async () => {
    const user = userEvent.setup();
    const onRequestPayment = vi.fn(noopAsync);
    const access: HostedAccessStatus = {
      ...baseAccess,
      available_plans: [
        {
          id: "trader",
          name: "Trader",
          price_isk: 300_000_000,
          period_days: 30,
          scan_limit_per_day: 250,
          features: ["basic_scans"],
        },
      ],
    };

    render(
      <HostedAccessTab
        access={access}
        loading={false}
        error={null}
        lastCheckedAt={null}
        onReload={() => {}}
        onRequestPayment={onRequestPayment}
        onMarkPaymentSent={noopAsync}
        onCancelPayment={noopAsync}
        formatIsk={(value) => `${value / 1_000_000}M`}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Trader/i }));
    expect(onRequestPayment).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /Create payment request for Trader/i }));
    expect(onRequestPayment).toHaveBeenCalledWith("trader");
  });

  it("hides pending payment actions once access is active", () => {
    const access: HostedAccessStatus = {
      ...baseAccess,
      status: "active",
      plan: { id: "trader", name: "Trader", expires_at: "2026-07-20T00:00:00Z" },
      payment: {
        receiver_name: "EVE Flipper Billing",
        receiver_character_id: 2124476406,
        amount_isk: 300_000_000,
        reason_code: "EFLIP-PENDING",
      },
      payment_history: [
        {
          code: "EFLIP-PENDING",
          plan_id: "trader",
          amount_isk: 300_000_000,
          status: "pending",
          created_at: "2026-06-20T00:00:00Z",
          expires_at: "2026-06-21T00:00:00Z",
        },
      ],
    };

    render(
      <HostedAccessTab
        access={access}
        loading={false}
        error={null}
        lastCheckedAt={null}
        onReload={() => {}}
        onRequestPayment={noopAsync}
        onMarkPaymentSent={noopAsync}
        onCancelPayment={noopAsync}
        formatIsk={(value) => `${value / 1_000_000}M`}
      />,
    );

    expect(screen.getByText("No pending payment. Current hosted access is active.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /I sent ISK/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Cancel pending/i })).not.toBeInTheDocument();
  });
});

describe("formatHostedPaymentCountdown", () => {
  it("formats future expiry relative to the supplied clock", () => {
    const now = Date.parse("2026-06-19T00:00:00Z");
    expect(formatHostedPaymentCountdown("2026-06-20T02:30:00Z", now)).toBe("1d 2h left");
    expect(formatHostedPaymentCountdown("2026-06-19T01:05:00Z", now)).toBe("1h 5m left");
  });

  it("handles expired or invalid expiry values", () => {
    const now = Date.parse("2026-06-19T00:00:00Z");
    expect(formatHostedPaymentCountdown("2026-06-18T23:59:59Z", now)).toBe("Expired");
    expect(formatHostedPaymentCountdown("not-a-date", now)).toBe("Unknown expiry");
  });
});
