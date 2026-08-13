import { twc } from "react-twc";
import type { RouterConfig } from "../api/index.ts";
import { useConfigQuery } from "../api/queries.ts";
import { DnsForm } from "./dns/DnsForm.tsx";

const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;

// Gate-only wrapper: DnsForm's useForm() must not mount until real config
// data exists, since its defaultValues are seeded once at mount (same
// pattern as NetworkPage gating Wan/Lan/MtuSection — see those files).
export function DnsPage() {
  const configQuery = useConfigQuery();
  const config: RouterConfig | undefined = configQuery.data;

  if (!config) {
    return <PageTitle>正在加载…</PageTitle>;
  }

  return <DnsForm config={config} />;
}
