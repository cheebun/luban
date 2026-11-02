import { useRateLimitedCallback } from "@tanstack/react-pacer";
import { useState } from "react";
import { twc } from "react-twc";

const ActionLink = twc.button`text-sm font-medium text-blue-600 hover:text-blue-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed`;
const Hint = twc.div`text-xs text-amber-600 mt-1`;

const RESTART_WINDOW_MS = 10_000;

export interface RestartRequest {
  name: string;
  // Runs the restart if the per-service rate limit allows it; returns false
  // (and shows the hint on the row) when limited.
  attempt: () => boolean;
}

interface Props {
  name: string;
  restarting: boolean;
  onRestart: (name: string) => void;
  onRequest: (req: RestartRequest) => void;
}

// One instance per service row, so the 1-per-10s limit is per service name.
export function ServiceRestartAction({ name, restarting, onRestart, onRequest }: Props) {
  const [limited, setLimited] = useState(false);
  const attempt = useRateLimitedCallback(
    () => {
      setLimited(false);
      onRestart(name);
    },
    {
      limit: 1,
      window: RESTART_WINDOW_MS,
      windowType: "fixed",
      onReject: () => setLimited(true),
    },
  );
  return (
    <>
      <ActionLink type="button" disabled={restarting} onClick={() => onRequest({ name, attempt })}>
        {restarting ? "重启中…" : "重启服务"}
      </ActionLink>
      {limited && <Hint>操作过于频繁，请稍后再试</Hint>}
    </>
  );
}
