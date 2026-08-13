// TanStack Mutation hooks for every write endpoint.
//
// Optimistic updates are scoped ONLY to saveConfig (used by both NetworkPage's
// three sections and DnsPage's upstream-list edits — both write through the
// same PUT /api/config call, so one hook covers "DNS-list edits" too).
// apply/confirm/rollback are deliberately left non-optimistic: ApplyDialog's
// countdown/polling/auto-confirm state machine must observe the real
// pending/success/error transitions, not an assumed optimistic result.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  applyConfig,
  changePassword,
  confirmApply,
  login,
  restartService,
  rollbackApply,
  saveConfig,
} from "./index.ts";
import { queryKeys } from "./queries.ts";
import type {
  ApplyRequest,
  LoginRequest,
  PasswordRequest,
  RouterConfig,
  ServiceRestartRequest,
} from "./types.ts";

export function useLoginMutation() {
  return useMutation({ mutationFn: (req: LoginRequest) => login(req) });
}

export function useChangePasswordMutation() {
  return useMutation({ mutationFn: (req: PasswordRequest) => changePassword(req) });
}

export function useSaveConfigMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (config: RouterConfig) => saveConfig(config),
    onMutate: async (config) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.config });
      const previous = queryClient.getQueryData<RouterConfig>(queryKeys.config);
      queryClient.setQueryData(queryKeys.config, config);
      return { previous };
    },
    onError: (_err, _config, context) => {
      if (context?.previous) queryClient.setQueryData(queryKeys.config, context.previous);
    },
    // Returning the invalidation promise keeps the mutation "pending" until
    // the refetch completes, per the official optimistic-updates guide.
    onSettled: () => queryClient.invalidateQueries({ queryKey: queryKeys.config }),
  });
}

// Not optimistic — ApplyDialog needs the real pending/success/error state.
export function useApplyMutation() {
  return useMutation({ mutationFn: (req?: ApplyRequest) => applyConfig(req) });
}

export function useConfirmMutation() {
  return useMutation({ mutationFn: () => confirmApply() });
}

export function useRollbackMutation() {
  return useMutation({ mutationFn: () => rollbackApply() });
}

export function useRestartServiceMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: ServiceRestartRequest) => restartService(req),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.health }),
  });
}
