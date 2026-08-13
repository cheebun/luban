import { useCallback, useEffect, useRef } from "react";
import { twc } from "react-twc";
import { applyConfig, confirmApply, isNetworkError, probeReachable } from "../../api/index.ts";
import { ROLLBACK_SECS, useApplyDialogStore } from "../../store/applyDialogStore.ts";
import { Alert, AlertBody, AlertTitle, Button } from "../ui/index.ts";

const Backdrop = twc.div`
  fixed inset-0 z-50 flex items-center justify-center bg-black/50
`;

const DialogPanel = twc.div`
  w-full max-w-md rounded-xl bg-white p-6 shadow-xl
`;

const DialogTitle = twc.h2`text-lg font-semibold text-gray-900 mb-4`;

const CountdownBar = twc.div`h-2 rounded-full bg-gray-100 overflow-hidden mb-4`;

const CountdownFill = twc.div`h-full bg-blue-500 transition-all duration-1000`;

const StatusText = twc.p`text-sm text-gray-600 mb-4`;

const ButtonRow = twc.div`flex gap-3 justify-end`;

const POLL_INTERVAL_MS = 3000;

interface Props {
  open: boolean;
  unchecked: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export function ApplyDialog({ open, unchecked, onClose, onSuccess }: Props) {
  const state = useApplyDialogStore((s) => s.state);
  const errorMsg = useApplyDialogStore((s) => s.errorMsg);
  const secondsLeft = useApplyDialogStore((s) => s.secondsLeft);
  const setState = useApplyDialogStore((s) => s.setState);
  const setErrorMsg = useApplyDialogStore((s) => s.setErrorMsg);
  const setSecondsLeft = useApplyDialogStore((s) => s.setSecondsLeft);
  const resetDialogState = useApplyDialogStore((s) => s.reset);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // Mutual recursion (doConfirm resumes polling on a mid-window failure,
  // startPolling's tick calls doConfirm) is threaded through refs updated
  // every render rather than useCallback deps, so neither has to be defined
  // before the other.
  const secondsLeftRef = useRef(secondsLeft);
  secondsLeftRef.current = secondsLeft;
  const doConfirmRef = useRef<() => Promise<void>>(async () => {});
  const startPollingRef = useRef<() => void>(() => {});

  const clearTimers = useCallback(() => {
    if (pollRef.current) clearInterval(pollRef.current);
    if (countdownRef.current) clearInterval(countdownRef.current);
    pollRef.current = null;
    countdownRef.current = null;
  }, []);

  const doConfirm = useCallback(async () => {
    setState("confirming");
    clearTimers();
    try {
      await confirmApply();
      setState("confirmed");
      setTimeout(onSuccess, 1200);
    } catch {
      // Confirm can fail either because the router is still mid-restart
      // (network flap) or because a genuine backend error is now reachable.
      // Either way, as long as the rollback countdown hasn't expired the
      // config is still live and recoverable — resume polling instead of
      // surfacing a raw error. Only past countdown expiry do we give up and
      // tell the user the box may have already rolled back on its own.
      if (secondsLeftRef.current <= 0) {
        setState("error");
        setErrorMsg("可能已自动回滚，请刷新页面确认路由器状态");
      } else {
        startPollingRef.current();
      }
    }
  }, [clearTimers, onSuccess, setState, setErrorMsg]);
  doConfirmRef.current = doConfirm;

  const startPolling = useCallback(() => {
    setState("polling");

    countdownRef.current = setInterval(() => {
      setSecondsLeft((s) => {
        if (s <= 1) {
          clearTimers();
          setState("timeout");
          return 0;
        }
        return s - 1;
      });
    }, 1000);

    pollRef.current = setInterval(async () => {
      const reachable = await probeReachable("/api/status");
      if (reachable) await doConfirmRef.current();
      // still unreachable — keep polling
    }, POLL_INTERVAL_MS);
  }, [clearTimers, setState, setSecondsLeft]);
  startPollingRef.current = startPolling;

  useEffect(() => {
    if (!open) return;

    resetDialogState();
    clearTimers();

    let cancelled = false;

    void applyConfig({ unchecked })
      .then(() => {
        if (cancelled) return;
        if (unchecked) {
          setState("confirmed");
          setTimeout(onSuccess, 800);
        } else {
          startPolling();
        }
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        // A network-level failure on apply is the EXPECTED success path:
        // the backend restarts caddy/networkd as part of applying, which
        // kills the in-flight request. Treat it exactly like a 200 and
        // fall into the same poll/confirm/countdown flow. Only an HTTP
        // error response (validation/render failure with a body) is a real
        // error here.
        if (isNetworkError(err)) {
          startPolling();
          return;
        }
        setState("error");
        setErrorMsg(err instanceof Error ? err.message : "应用失败");
      });

    return () => {
      cancelled = true;
      clearTimers();
    };
  }, [open, unchecked, clearTimers, startPolling, onSuccess, resetDialogState, setState, setErrorMsg]);

  if (!open) return null;

  const pct = Math.round((secondsLeft / ROLLBACK_SECS) * 100);

  return (
    <Backdrop>
      <DialogPanel>
        <DialogTitle>正在应用配置</DialogTitle>

        {(state === "applying" || state === "confirming") && (
          <StatusText>
            {state === "applying" ? "正在推送配置，请稍候…" : "正在确认配置…"}
          </StatusText>
        )}

        {state === "polling" && (
          <>
            <CountdownBar>
              <CountdownFill style={{ width: `${pct}%` }} />
            </CountdownBar>
            <StatusText>正在等待路由器恢复，{secondsLeft} 秒后自动回滚</StatusText>
          </>
        )}

        {state === "timeout" && (
          <>
            <Alert $type="warning" className="mb-4">
              <AlertTitle>连接超时</AlertTitle>
              <AlertBody>
                未能自动确认，如果配置正常请手动确认。如不确认，将在回滚计时器到期时自动恢复旧配置。
              </AlertBody>
            </Alert>
            <ButtonRow>
              <Button $variant="secondary" onClick={onClose}>
                关闭
              </Button>
              <Button onClick={() => void doConfirm()}>手动确认</Button>
            </ButtonRow>
          </>
        )}

        {state === "confirmed" && (
          <Alert $type="success">
            <AlertTitle>配置已确认</AlertTitle>
            <AlertBody>路由器配置已成功应用并确认。</AlertBody>
          </Alert>
        )}

        {state === "error" && (
          <>
            <Alert $type="error" className="mb-4">
              <AlertTitle>错误</AlertTitle>
              <AlertBody>{errorMsg}</AlertBody>
            </Alert>
            <ButtonRow>
              <Button $variant="secondary" onClick={onClose}>
                关闭
              </Button>
            </ButtonRow>
          </>
        )}
      </DialogPanel>
    </Backdrop>
  );
}
