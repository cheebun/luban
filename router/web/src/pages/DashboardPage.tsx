import { useConfigQuery, useStatusQuery } from "../api/queries.ts";
import { InterfacesAndLeases } from "./dashboard/InterfacesAndLeases.tsx";
import { OverviewCards } from "./dashboard/OverviewCards.tsx";
import { GridLayout, PageTitle, RefreshNote, RefreshRow } from "./dashboard/primitives.tsx";

export function DashboardPage() {
  const statusQuery = useStatusQuery();
  const configQuery = useConfigQuery();
  const status = statusQuery.data;
  const config = configQuery.data;
  const lastUpdated = statusQuery.dataUpdatedAt ? new Date(statusQuery.dataUpdatedAt) : null;

  if (!status || !config) {
    return <PageTitle>正在加载…</PageTitle>;
  }

  return (
    <>
      <RefreshRow>
        <PageTitle>概述</PageTitle>
        <RefreshNote>
          {lastUpdated ? `最后更新: ${lastUpdated.toLocaleTimeString("zh-CN")}` : ""}
        </RefreshNote>
      </RefreshRow>

      <GridLayout>
        <OverviewCards status={status} config={config} />
        <InterfacesAndLeases status={status} />
      </GridLayout>
    </>
  );
}
