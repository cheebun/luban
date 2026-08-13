import { Badge, Card, CardBody, CardHeader, CardTitle } from "../../components/ui/index.ts";
import type { RouterConfig, ServiceHealth, StatusResponse } from "../../api/index.ts";
import {
  formatBytes,
  formatLoadAvg,
  formatTemperature,
  formatUptime,
  findWanIp,
  orUnknown,
  SERVICE_STATUS,
  UNKNOWN,
  WAN_MODES,
} from "./format.ts";
import { DlGrid, StatLabel, StatValue, Table, Td, Th, Thead, TableWrapper } from "./primitives.tsx";

function ServiceRow({ svc }: { svc: ServiceHealth }) {
  const label =
    svc.active === "active" ? "运行中" : svc.active === "inactive" ? "已停止" : svc.active;
  return (
    <tr>
      <Td>{svc.name}</Td>
      <Td>
        <Badge $status={SERVICE_STATUS[svc.active] ?? "warning"}>{label}</Badge>
      </Td>
    </tr>
  );
}

interface Props {
  status: StatusResponse;
  config: RouterConfig;
}

// WAN/LAN/Services/Host/Hardware summary cards — the first "row" of the
// overview grid. Split out of DashboardPage to keep that file under the
// 300-line cap.
export function OverviewCards({ status, config }: Props) {
  const wanIp = findWanIp(status.addrs, config.wan.interface);

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>WAN 状态</CardTitle>
        </CardHeader>
        <CardBody>
          <DlGrid>
            <div>
              <StatLabel>模式</StatLabel>
              <StatValue>{WAN_MODES[config.wan.mode] ?? config.wan.mode}</StatValue>
            </div>
            <div>
              <StatLabel>IP 地址</StatLabel>
              <StatValue>{wanIp ?? "—"}</StatValue>
            </div>
            <div>
              <StatLabel>运行时间</StatLabel>
              <StatValue>{formatUptime(status.uptime)}</StatValue>
            </div>
            <div>
              <StatLabel>连接状态</StatLabel>
              <StatValue>
                <Badge $status={wanIp ? "online" : "offline"}>{wanIp ? "已连接" : "未连接"}</Badge>
              </StatValue>
            </div>
            <div>
              <StatLabel>网关 (IPv4)</StatLabel>
              <StatValue>{orUnknown(status.system.gateway_v4)}</StatValue>
            </div>
            <div>
              <StatLabel>网关 (IPv6)</StatLabel>
              <StatValue>{orUnknown(status.system.gateway_v6)}</StatValue>
            </div>
          </DlGrid>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>LAN 信息</CardTitle>
        </CardHeader>
        <CardBody>
          <DlGrid>
            <div>
              <StatLabel>LAN 地址</StatLabel>
              <StatValue>{config.lan.address}</StatValue>
            </div>
            {config.ipv6.enabled && (
              <div>
                <StatLabel>IPv6</StatLabel>
                <StatValue>已启用</StatValue>
              </div>
            )}
          </DlGrid>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>服务状态</CardTitle>
        </CardHeader>
        <CardBody>
          <TableWrapper>
            <Table>
              <Thead>
                <tr>
                  <Th>服务</Th>
                  <Th>状态</Th>
                </tr>
              </Thead>
              <tbody>
                {status.services.map((svc) => (
                  <ServiceRow key={svc.name} svc={svc} />
                ))}
              </tbody>
            </Table>
          </TableWrapper>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>主机信息</CardTitle>
        </CardHeader>
        <CardBody>
          <DlGrid>
            <div>
              <StatLabel>主机名</StatLabel>
              <StatValue>{orUnknown(status.system.hostname)}</StatValue>
            </div>
            <div>
              <StatLabel>系统版本</StatLabel>
              <StatValue>{orUnknown(status.system.os)}</StatValue>
            </div>
            <div>
              <StatLabel>内核</StatLabel>
              <StatValue>{orUnknown(status.system.kernel)}</StatValue>
            </div>
            <div>
              <StatLabel>架构</StatLabel>
              <StatValue>{orUnknown(status.system.arch)}</StatValue>
            </div>
            <div>
              <StatLabel>运行时长</StatLabel>
              <StatValue>{formatUptime(status.system.uptime)}</StatValue>
            </div>
            <div>
              <StatLabel>负载 (1/5/15)</StatLabel>
              <StatValue>{formatLoadAvg(status.system.loadavg)}</StatValue>
            </div>
          </DlGrid>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>硬件概览</CardTitle>
        </CardHeader>
        <CardBody>
          <DlGrid>
            <div>
              <StatLabel>CPU</StatLabel>
              <StatValue>
                {status.system.cpu
                  ? `${orUnknown(status.system.cpu.model)} × ${status.system.cpu.cores}`
                  : UNKNOWN}
              </StatValue>
            </div>
            <div>
              <StatLabel>CPU 温度</StatLabel>
              <StatValue>{formatTemperature(status.system.cpu?.temperature_c)}</StatValue>
            </div>
            <div>
              <StatLabel>内存</StatLabel>
              <StatValue>
                {status.system.memory
                  ? `${formatBytes(status.system.memory.total_bytes - status.system.memory.available_bytes)} / ${formatBytes(status.system.memory.total_bytes)}`
                  : UNKNOWN}
              </StatValue>
            </div>
            <div>
              <StatLabel>磁盘</StatLabel>
              <StatValue>
                {status.system.disk
                  ? `${formatBytes(status.system.disk.total_bytes - status.system.disk.free_bytes)} / ${formatBytes(status.system.disk.total_bytes)}`
                  : UNKNOWN}
              </StatValue>
            </div>
          </DlGrid>
        </CardBody>
      </Card>
    </>
  );
}
