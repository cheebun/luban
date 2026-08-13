import { Badge, Card, CardBody, CardHeader, CardTitle } from "../../components/ui/index.ts";
import type { InterfaceInfo, StatusResponse } from "../../api/index.ts";
import { formatLinkSpeed, IFACE_STATE_BADGE, IFACE_TYPE_LABELS } from "./format.ts";
import {
  Chip,
  ChipRow,
  MutedNote,
  Table,
  TableWrapper,
  Td,
  Th,
  Thead,
} from "./primitives.tsx";

function InterfaceRow({ iface }: { iface: InterfaceInfo }) {
  return (
    <tr>
      <Td>{IFACE_TYPE_LABELS[iface.type]}</Td>
      <Td>{iface.type}</Td>
      <Td className="font-mono text-xs">{iface.name}</Td>
      <Td className="font-mono text-xs">{iface.mac || "—"}</Td>
      <Td className="font-mono text-xs">{iface.ipv4.length > 0 ? iface.ipv4.join(", ") : "—"}</Td>
      <Td className="font-mono text-xs">{iface.ipv6.length > 0 ? iface.ipv6.join(", ") : "—"}</Td>
      <Td>{formatLinkSpeed(iface.link_speed_mbps)}</Td>
      <Td>
        <Badge $status={IFACE_STATE_BADGE[iface.state] ?? "warning"}>{iface.state}</Badge>
      </Td>
    </tr>
  );
}

interface Props {
  status: StatusResponse;
}

// 本机 IP / 活动连接 / DHCP 租约 cards — split out of DashboardPage to keep
// that file under the 300-line cap.
export function InterfacesAndLeases({ status }: Props) {
  const leases = status.leases;

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>本机 IP</CardTitle>
        </CardHeader>
        <CardBody>
          {status.system.interfaces.some((i) => i.ipv4.length > 0 || i.ipv6.length > 0) ? (
            <ChipRow>
              {status.system.interfaces.flatMap((iface) =>
                [...iface.ipv4, ...iface.ipv6].map((addr) => (
                  <Chip key={`${iface.name}-${addr}`}>{addr}</Chip>
                )),
              )}
            </ChipRow>
          ) : (
            <MutedNote>暂无地址</MutedNote>
          )}
        </CardBody>
      </Card>

      <Card className="md:col-span-2 xl:col-span-3">
        <CardHeader>
          <CardTitle>活动连接</CardTitle>
        </CardHeader>
        <CardBody>
          <TableWrapper>
            <Table>
              <Thead>
                <tr>
                  <Th>名称</Th>
                  <Th>类型</Th>
                  <Th>设备</Th>
                  <Th>MAC</Th>
                  <Th>IPv4</Th>
                  <Th>IPv6</Th>
                  <Th>协商速率</Th>
                  <Th>状态</Th>
                </tr>
              </Thead>
              <tbody>
                {status.system.interfaces.map((iface) => (
                  <InterfaceRow key={iface.name} iface={iface} />
                ))}
              </tbody>
            </Table>
          </TableWrapper>
        </CardBody>
      </Card>

      {leases.length > 0 && (
        <Card className="md:col-span-2 xl:col-span-3">
          <CardHeader>
            <CardTitle>DHCP 租约 ({leases.length})</CardTitle>
          </CardHeader>
          <CardBody>
            <TableWrapper>
              <Table>
                <Thead>
                  <tr>
                    <Th>主机名</Th>
                    <Th>IP 地址</Th>
                    <Th>MAC 地址</Th>
                    <Th>到期时间</Th>
                  </tr>
                </Thead>
                <tbody>
                  {leases.map((lease) => (
                    <tr key={lease.mac}>
                      <Td>{lease.hostname || "—"}</Td>
                      <Td>{lease.ip}</Td>
                      <Td className="font-mono text-xs">{lease.mac}</Td>
                      <Td>
                        {lease.expiry ? new Date(lease.expiry * 1000).toLocaleString("zh-CN") : "—"}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </TableWrapper>
          </CardBody>
        </Card>
      )}
    </>
  );
}
