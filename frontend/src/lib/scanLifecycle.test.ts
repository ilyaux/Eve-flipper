import { describe, expect, it } from "vitest";
import {
  invalidateScanRequest,
  isCurrentScanRequest,
  startScanRequest,
  type ScanLifecycleState,
} from "./scanLifecycle";

describe("scan lifecycle guard", () => {
  it("invalidates stale scan responses after a newer scan starts", () => {
    const state: ScanLifecycleState<AbortController> = {
      currentRequestId: 0,
      currentController: null,
    };
    const first = new AbortController();
    const firstId = startScanRequest(state, first);

    expect(isCurrentScanRequest(state, firstId, first)).toBe(true);

    const second = new AbortController();
    const secondId = startScanRequest(state, second);

    expect(isCurrentScanRequest(state, firstId, first)).toBe(false);
    expect(isCurrentScanRequest(state, secondId, second)).toBe(true);
  });

  it("invalidates aborted scans before stale finally handlers can reset state", () => {
    const state: ScanLifecycleState<AbortController> = {
      currentRequestId: 0,
      currentController: null,
    };
    const controller = new AbortController();
    const requestId = startScanRequest(state, controller);

    invalidateScanRequest(state);
    controller.abort();

    expect(isCurrentScanRequest(state, requestId, controller)).toBe(false);
    expect(state.currentController).toBeNull();
  });
});
