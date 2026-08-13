import { useState } from "react";
import { twc } from "react-twc";
import { probeWizard } from "../../api/index.ts";
import type { WizardState } from "../../api/index.ts";
import { Alert, AlertBody, Badge, Button } from "../../components/ui/index.ts";

const SectionTitle = twc.h3`text-sm font-semibold text-gray-700 mb-2`;
const BoardName = twc.p`text-base font-medium text-gray-900 mb-4`;

const Table = twc.table`w-full text-sm border-collapse`;
const Th = twc.th`text-left text-xs font-medium text-gray-500 px-3 py-2 border-b border-gray-200`;
const Td = twc.td`px-3 py-2 border-b border-gray-100`;

const FooterRow = twc.div`flex items-center justify-between mt-6`;

function roleBadge(role: "wan" | "lan" | null) {
  if (role === "wan") return <Badge $status="warning">WAN</Badge>;
  if (role === "lan") return <Badge $status="online">LAN</Badge>;
  return <span className="text-gray-400">—</span>;
}

function dhcpCell(offer: boolean | null, server: string | null) {
  if (offer === null) return <span className="text-gray-400">未探测</span>;
  if (!offer) return <span className="text-gray-400">无</span>;
  return <span className="text-green-700">{server ?? "有"}</span>;
}

interface Props {
  state: WizardState;
  onStateUpdate: (updater: (prev: WizardState) => WizardState) => void;
  onNext: () => void;
  onBack: () => void;
}

export function HardwareStep({ state, onStateUpdate, onNext, onBack }: Props) {
  const [probing, setProbing] = useState(false);
  const [probeError, setProbeError] = useState("");

  async function handleProbe() {
    setProbing(true);
    setProbeError("");
    try {
      const result = await probeWizard();
      onStateUpdate((prev) => ({ ...prev, interfaces: result.interfaces, probed: true }));
    } catch {
      setProbeError("探测失败，请重试");
    } finally {
      setProbing(false);
    }
  }

  return (
    <div>
      <SectionTitle>板卡型号</SectionTitle>
      <BoardName>
        {state.board ? state.board.name : "未识别的设备"}
      </BoardName>

      <SectionTitle>网络接口</SectionTitle>
      {state.interfaces.length === 0 ? (
        <Alert $type="warning" className="mb-4">
          <AlertBody>未检测到网络接口，请点击「重新探测」。</AlertBody>
        </Alert>
      ) : (
        <div className="overflow-x-auto mb-4">
          <Table>
            <thead>
              <tr>
                <Th>接口</Th>
                <Th>位置</Th>
                <Th>MAC</Th>
                <Th>链路</Th>
                <Th>角色建议</Th>
                <Th>DHCP 探测</Th>
              </tr>
            </thead>
            <tbody>
              {state.interfaces.map((iface) => (
                <tr key={iface.name}>
                  <Td className="font-mono">{iface.name}</Td>
                  <Td className="text-gray-500 text-xs">{iface.path || "—"}</Td>
                  <Td className="font-mono text-xs">{iface.mac}</Td>
                  <Td>
                    {iface.link ? (
                      <Badge $status="online">已连接</Badge>
                    ) : (
                      <Badge $status="offline">断开</Badge>
                    )}
                  </Td>
                  <Td>{roleBadge(iface.role_suggestion)}</Td>
                  <Td>{dhcpCell(iface.dhcp_offer, iface.offer_server)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      )}

      {probeError && (
        <Alert $type="error" className="mb-4">
          <AlertBody>{probeError}</AlertBody>
        </Alert>
      )}

      {probing && (
        <Alert $type="info" className="mb-4">
          <AlertBody>正在探测接口（约 10 秒），请稍候…</AlertBody>
        </Alert>
      )}

      <FooterRow>
        <div className="flex gap-2">
          <Button $variant="secondary" onClick={onBack}>
            上一步
          </Button>
          <Button $variant="secondary" onClick={() => void handleProbe()} disabled={probing}>
            {probing ? "探测中…" : "重新探测"}
          </Button>
        </div>
        <Button onClick={onNext} disabled={state.interfaces.length === 0 || probing}>
          下一步
        </Button>
      </FooterRow>
    </div>
  );
}
