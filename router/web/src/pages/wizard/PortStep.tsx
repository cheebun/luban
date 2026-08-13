import { twc } from "react-twc";
import type { WizardInterface } from "../../api/index.ts";
import { Alert, AlertBody, Button, FormGroup, Label, Select } from "../../components/ui/index.ts";
import type { WizardFormData } from "./WizardPage.tsx";

const FooterRow = twc.div`flex items-center justify-between mt-6`;
const CheckRow = twc.label`flex items-center gap-2 text-sm text-gray-700 cursor-pointer py-1`;

interface Props {
  interfaces: WizardInterface[];
  formData: WizardFormData;
  onUpdate: (updates: Partial<WizardFormData>) => void;
  onNext: () => void;
  onBack: () => void;
}

export function PortStep({ interfaces, formData, onUpdate, onNext, onBack }: Props) {
  const wanIface = formData.wanInterface;

  function toggleLan(name: string, checked: boolean) {
    const next = checked
      ? [...formData.lanInterfaces, name]
      : formData.lanInterfaces.filter((n) => n !== name);
    onUpdate({ lanInterfaces: next });
  }

  const singlePort = interfaces.length === 1;
  const noLan = formData.lanInterfaces.length === 0;
  const canProceed = wanIface !== "";

  return (
    <div>
      <FormGroup>
        <Label htmlFor="wan-select">WAN 接口（上行）</Label>
        <Select
          id="wan-select"
          value={wanIface}
          onChange={(e) => {
            const next = e.target.value;
            onUpdate({
              wanInterface: next,
              // drop from LAN if was selected there
              lanInterfaces: formData.lanInterfaces.filter((n) => n !== next),
            });
          }}
        >
          <option value="">— 请选择 WAN 接口 —</option>
          {interfaces.map((iface) => (
            <option key={iface.name} value={iface.name}>
              {iface.name}
              {iface.link ? " (已连接)" : ""}
              {iface.role_suggestion === "wan" ? " ★" : ""}
            </option>
          ))}
        </Select>
      </FormGroup>

      <FormGroup>
        <Label>LAN 接口（内网，可多选）</Label>
        <div className="border border-gray-200 rounded-md px-3 py-2">
          {interfaces.filter((iface) => iface.name !== wanIface).length === 0 ? (
            <p className="text-sm text-gray-400 py-1">选择 WAN 接口后此列表显示剩余接口</p>
          ) : (
            interfaces
              .filter((iface) => iface.name !== wanIface)
              .map((iface) => (
                <CheckRow key={iface.name}>
                  <input
                    type="checkbox"
                    className="h-4 w-4 rounded border-gray-300 text-blue-600"
                    checked={formData.lanInterfaces.includes(iface.name)}
                    onChange={(e) => toggleLan(iface.name, e.target.checked)}
                  />
                  <span className="font-mono">{iface.name}</span>
                  {iface.link && <span className="text-xs text-green-600">已连接</span>}
                  {iface.role_suggestion === "lan" && (
                    <span className="text-xs text-gray-400">（建议）</span>
                  )}
                </CheckRow>
              ))
          )}
        </div>
      </FormGroup>

      {noLan && !singlePort && (
        <Alert $type="warning" className="mb-4">
          <AlertBody>未选择任何 LAN 接口，路由器将没有内网服务。建议至少选择一个。</AlertBody>
        </Alert>
      )}
      {noLan && singlePort && (
        <Alert $type="info" className="mb-4">
          <AlertBody>单端口设备：该接口用作 WAN，无 LAN 接口，仅限直连管理访问。</AlertBody>
        </Alert>
      )}

      <FooterRow>
        <Button $variant="secondary" onClick={onBack}>
          上一步
        </Button>
        <Button onClick={onNext} disabled={!canProceed}>
          下一步
        </Button>
      </FooterRow>
    </div>
  );
}
