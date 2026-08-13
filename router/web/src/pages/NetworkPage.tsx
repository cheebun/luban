import { useState } from "react";
import { useConfigQuery } from "../api/queries.ts";
import { ApplyDialog } from "../components/apply/ApplyDialog.tsx";
import { LanSection } from "./network/LanSection.tsx";
import { MtuSection } from "./network/MtuSection.tsx";
import { PageTitle, SectionStack } from "./network/primitives.tsx";
import { WanSection } from "./network/WanSection.tsx";

// Orchestrator only: each section owns its own TanStack Form instance and
// saveConfig submit, and reports back here just to open the shared
// ApplyDialog (which owns its own apply/poll/confirm/rollback state machine
// unrelated to any single section's form state).
export function NetworkPage() {
  const configQuery = useConfigQuery();
  const [applyOpen, setApplyOpen] = useState(false);
  const [activeUnchecked, setActiveUnchecked] = useState(false);

  const config = configQuery.data;
  if (!config) {
    return <PageTitle>正在加载…</PageTitle>;
  }

  function handleApplied(unchecked: boolean) {
    setActiveUnchecked(unchecked);
    setApplyOpen(true);
  }

  return (
    <>
      <PageTitle>网络设置</PageTitle>

      <SectionStack>
        <WanSection config={config} onApplied={handleApplied} />
        <LanSection config={config} onApplied={handleApplied} />
        {config.wan.mode !== "bridge" && <MtuSection config={config} onApplied={handleApplied} />}
      </SectionStack>

      <ApplyDialog
        open={applyOpen}
        unchecked={activeUnchecked}
        onClose={() => setApplyOpen(false)}
        onSuccess={() => setApplyOpen(false)}
      />
    </>
  );
}
