export interface ScanLifecycleState<TController> {
  currentRequestId: number;
  currentController: TController | null;
}

export function startScanRequest<TController>(
  state: ScanLifecycleState<TController>,
  controller: TController,
): number {
  const requestId = state.currentRequestId + 1;
  state.currentRequestId = requestId;
  state.currentController = controller;
  return requestId;
}

export function invalidateScanRequest<TController>(state: ScanLifecycleState<TController>) {
  state.currentRequestId += 1;
  state.currentController = null;
}

export function isCurrentScanRequest<TController extends { signal?: { aborted?: boolean } }>(
  state: ScanLifecycleState<TController>,
  requestId: number,
  controller: TController,
): boolean {
  return (
    state.currentRequestId === requestId &&
    state.currentController === controller &&
    !controller.signal?.aborted
  );
}
