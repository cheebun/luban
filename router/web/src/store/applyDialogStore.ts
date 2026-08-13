// Client-only state machine for ApplyDialog (applying/polling/confirming/
// confirmed/timeout/error + the rollback countdown). Previously three
// useState hooks inside the component; moved to zustand per the state/data
// stack decision (see DECISIONS.md "Web Stack" addendum) while leaving the
// actual async orchestration (timers, polling loop, mutation calls) in
// ApplyDialog.tsx untouched — this store only holds state, it does not
// perform the apply/confirm calls itself.
import { create } from "zustand";

export type ApplyState =
  | "applying"
  | "polling"
  | "confirming"
  | "confirmed"
  | "timeout"
  | "error";

export const ROLLBACK_SECS = 90;

interface ApplyDialogStoreState {
  state: ApplyState;
  errorMsg: string;
  secondsLeft: number;
  setState: (state: ApplyState) => void;
  setErrorMsg: (msg: string) => void;
  setSecondsLeft: (updater: number | ((prev: number) => number)) => void;
  /** Re-arms the dialog for a fresh apply cycle (called when `open` flips true). */
  reset: () => void;
}

export const useApplyDialogStore = create<ApplyDialogStoreState>((set) => ({
  state: "applying",
  errorMsg: "",
  secondsLeft: ROLLBACK_SECS,
  setState: (state) => set({ state }),
  setErrorMsg: (errorMsg) => set({ errorMsg }),
  setSecondsLeft: (updater) =>
    set((s) => ({
      secondsLeft: typeof updater === "function" ? updater(s.secondsLeft) : updater,
    })),
  reset: () => set({ state: "applying", errorMsg: "", secondsLeft: ROLLBACK_SECS }),
}));
