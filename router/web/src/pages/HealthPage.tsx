import { useCallback, useState } from "react";
import { twc } from "react-twc";
import { useHealthQuery } from "../api/queries.ts";
import { useRestartServiceMutation } from "../api/mutations.ts";
import type { HealthComponent, HealthService, HealthTakeoverCheck } from "../api/index.ts";
import {
  Alert,
  AlertBody,
  AlertTitle,
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
} from "../components/ui/index.ts";

const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;

const SectionStack = twc.div`flex flex-col gap-4`;

const TableWrapper = twc.div`overflow-x-auto`;

const Table = twc.table`min-w-full text-sm`;

const Thead = twc.thead`border-b border-gray-200`;

const Th = twc.th`text-left text-xs font-medium text-gray-500 uppercase tracking-wide pb-2 pr-4`;

const Td = twc.td`py-2 pr-4 text-gray-700 align-top`;

const ActionLink = twc.button`text-sm font-medium text-blue-600 hover:text-blue-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed`;

const RefreshRow = twc.div`flex items-center justify-between mb-6`;

const RefreshNote = twc.span`text-xs text-gray-400`;

const Backdrop = twc.div`fixed inset-0 z-50 flex items-center justify-center bg-black/50`;

const DialogPanel = twc.div`w-full max-w-sm rounded-xl bg-white p-6 shadow-xl`;

const DialogTitle = twc.h2`text-lg font-semibold text-gray-900 mb-4`;

const DialogBody = twc.p`text-sm text-gray-600 mb-4`;

const ButtonRow = twc.div`flex gap-3 justify-end`;

const OK = "无异常";
const NG = "存在问题";

function componentBadge(c: HealthComponent) {
  return c.installed ? (
    <Badge $status="online">已安装</Badge>
  ) : (
    <Badge $status="error">未安装</Badge>
  );
}

const SERVICE_ACTIVE_LABEL: Record<string, string> = {
  active: "运行中",
  inactive: "已停止",
  failed: "失败",
};

function serviceBadge(active: string) {
  const status = active === "active" ? "online" : active === "failed" ? "error" : "offline";
  return <Badge $status={status}>{SERVICE_ACTIVE_LABEL[active] ?? active}</Badge>;
}

function takeoverBadge(ok: boolean) {
  return ok ? <Badge $status="online">{OK}</Badge> : <Badge $status="error">{NG}</Badge>;
}

interface RestartTarget {
  name: string;
}

export function HealthPage() {
  const healthQuery = useHealthQuery();
  const restartMutation = useRestartServiceMutation();
  const health = healthQuery.data;
  const lastUpdated = healthQuery.dataUpdatedAt ? new Date(healthQuery.dataUpdatedAt) : null;
  const [confirmTarget, setConfirmTarget] = useState<RestartTarget | null>(null);
  const [restarting, setRestarting] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const doRestart = useCallback(
    async (name: string) => {
      setConfirmTarget(null);
      setRestarting(name);
      setError(null);
      try {
        await restartMutation.mutateAsync({ name });
        // router-ui restarts itself with a short delay server-side; give every
        // target a moment before refetching so the new state is observable.
        // The mutation's onSuccess already invalidates the health query —
        // this explicit refetch just waits for fresh data before clearing
        // the "restarting" indicator.
        await new Promise((resolve) => setTimeout(resolve, 1500));
        await healthQuery.refetch();
      } catch (err) {
        setError(err instanceof Error ? err.message : "重启失败");
      } finally {
        setRestarting(null);
      }
    },
    [restartMutation, healthQuery],
  );

  if (!health) {
    return <PageTitle>正在加载…</PageTitle>;
  }

  return (
    <>
      <RefreshRow>
        <PageTitle>系统自检</PageTitle>
        <RefreshNote>
          {lastUpdated ? `最后更新: ${lastUpdated.toLocaleTimeString("zh-CN")}` : ""}
        </RefreshNote>
      </RefreshRow>

      {error && (
        <Alert $type="error" className="mb-4">
          <AlertTitle>操作失败</AlertTitle>
          <AlertBody>{error}</AlertBody>
        </Alert>
      )}

      <SectionStack>
        <Card>
          <CardHeader>
            <CardTitle>组件</CardTitle>
          </CardHeader>
          <CardBody>
            <TableWrapper>
              <Table>
                <Thead>
                  <tr>
                    <Th>组件</Th>
                    <Th>状态</Th>
                    <Th>详情</Th>
                  </tr>
                </Thead>
                <tbody>
                  {health.components.map((c) => (
                    <tr key={c.name}>
                      <Td>{c.name}</Td>
                      <Td>{componentBadge(c)}</Td>
                      <Td>{c.version ? `${c.detail} (${c.version})` : c.detail}</Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </TableWrapper>
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>服务</CardTitle>
          </CardHeader>
          <CardBody>
            <TableWrapper>
              <Table>
                <Thead>
                  <tr>
                    <Th>服务</Th>
                    <Th>状态</Th>
                    <Th>详情</Th>
                    <Th>处置</Th>
                  </tr>
                </Thead>
                <tbody>
                  {health.services.map((s: HealthService) => (
                    <tr key={s.name}>
                      <Td>{s.name}</Td>
                      <Td>{serviceBadge(s.active)}</Td>
                      <Td>{s.detail || "—"}</Td>
                      <Td>
                        {s.restartable ? (
                          <ActionLink
                            type="button"
                            disabled={restarting === s.name}
                            onClick={() => setConfirmTarget({ name: s.name })}
                          >
                            {restarting === s.name ? "重启中…" : "重启服务"}
                          </ActionLink>
                        ) : (
                          "—"
                        )}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </TableWrapper>
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>接管检查</CardTitle>
          </CardHeader>
          <CardBody>
            <TableWrapper>
              <Table>
                <Thead>
                  <tr>
                    <Th>检查项</Th>
                    <Th>状态</Th>
                    <Th>详情</Th>
                  </tr>
                </Thead>
                <tbody>
                  {health.takeover.map((t: HealthTakeoverCheck) => (
                    <tr key={t.name}>
                      <Td>{t.name}</Td>
                      <Td>{takeoverBadge(t.ok)}</Td>
                      <Td>{t.detail}</Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </TableWrapper>
          </CardBody>
        </Card>
      </SectionStack>

      {confirmTarget && (
        <Backdrop>
          <DialogPanel>
            <DialogTitle>确认重启服务</DialogTitle>
            <DialogBody>
              {confirmTarget.name === "router-ui"
                ? "即将重启管理界面自身，页面可能短暂断开，请稍后刷新。确定继续？"
                : `即将重启服务「${confirmTarget.name}」，确定继续？`}
            </DialogBody>
            <ButtonRow>
              <Button $variant="secondary" onClick={() => setConfirmTarget(null)}>
                取消
              </Button>
              <Button $variant="danger" onClick={() => void doRestart(confirmTarget.name)}>
                确认重启
              </Button>
            </ButtonRow>
          </DialogPanel>
        </Backdrop>
      )}
    </>
  );
}
