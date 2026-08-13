import { useEffect, useRef, useState } from "react";
import { twc } from "react-twc";
import { completeWizard, isNetworkError, probeReachable } from "../../api/index.ts";
import type { WizardInterface } from "../../api/index.ts";
import { Alert, AlertBody, AlertTitle, Button } from "../../components/ui/index.ts";
import { maskToPrefix } from "../../lib/network.ts";
import type { WizardFormData } from "./WizardPage.tsx";

const FooterRow = twc.div`flex items-center justify-between mt-6`;
const SummaryTable = twc.table`w-full text-sm mb-6`;
const SummaryTh = twc.th`text-left text-xs font-medium text-gray-500 pb-2 pr-4 w-1/3`;
const SummaryTd = twc.td`text-gray-900 pb-2 font-mono`;

type ApplyState = "idle" | "applying" | "polling" | "success" | "error";

const POLL_INTERVAL_MS = 3000;

interface Props {
  formData: WizardFormData;
  interfaces: WizardInterface[];
  onBack: () => void;
}

export function ApplyStep({ formData, interfaces, onBack }: Props) {
  const [state, setState] = useState<ApplyState>("idle");
  const [errorMsg, setErrorMsg] = useState("");
  const [newUrl, setNewUrl] = useState("");
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  function computeFallbackUrl(lanIp: string): string {
    const port = window.location.port ? `:${window.location.port}` : "";
    return `${window.location.protocol}//${lanIp}${port}`;
  }

  function startPolling(targetUrl: string) {
    setState("polling");
    pollRef.current = setInterval(async () => {
      const reachable = await probeReachable(`${targetUrl}/api/status`);
      if (reachable) {
        if (pollRef.current) clearInterval(pollRef.current);
        setState("success");
        setTimeout(() => {
          window.location.href = targetUrl;
        }, 1500);
      }
    }, POLL_INTERVAL_MS);
  }

  async function handleApply() {
    setState("applying");
    setErrorMsg("");

    const prefix = maskToPrefix(formData.lanMask) ?? 24;
    const req = {
      wan_interface: formData.wanInterface,
      lan_interfaces: formData.lanInterfaces,
      lan_address: `${formData.lanIp}/${prefix}`,
      password: formData.password,
    };
    const fallbackUrl = computeFallbackUrl(formData.lanIp);

    try {
      const resp = await completeWizard(req);
      const target = resp.new_url ?? fallbackUrl;
      setNewUrl(target);
      startPolling(target);
    } catch (err) {
      if (isNetworkError(err)) {
        // Expected: backend restarted network mid-request. Treat as success and poll.
        setNewUrl(fallbackUrl);
        startPolling(fallbackUrl);
      } else {
        setState("error");
        setErrorMsg(err instanceof Error ? err.message : "应用配置失败");
      }
    }
  }

  const wanIface = interfaces.find((i) => i.name === formData.wanInterface);
  const lanIfaceNames = formData.lanInterfaces.join(", ") || "（无）";

  return (
    <div>
      <p className="text-sm text-gray-600 mb-4">请确认以下配置后点击「应用配置」。</p>

      <SummaryTable>
        <tbody>
          <tr>
            <SummaryTh>WAN 接口</SummaryTh>
            <SummaryTd>
              {formData.wanInterface}
              {wanIface?.link ? " (已连接)" : ""}
            </SummaryTd>
          </tr>
          <tr>
            <SummaryTh>LAN 接口</SummaryTh>
            <SummaryTd>{lanIfaceNames}</SummaryTd>
          </tr>
          <tr>
            <SummaryTh>LAN 地址</SummaryTh>
            <SummaryTd>
              {formData.lanIp}/{maskToPrefix(formData.lanMask) ?? "?"}
            </SummaryTd>
          </tr>
          <tr>
            <SummaryTh>管理员密码</SummaryTh>
            <SummaryTd>{"*".repeat(Math.min(formData.password.length, 10))}</SummaryTd>
          </tr>
        </tbody>
      </SummaryTable>

      {state === "applying" && (
        <Alert $type="info" className="mb-4">
          <AlertTitle>正在应用配置…</AlertTitle>
          <AlertBody>正在推送配置，路由器即将重启网络服务，请勿关闭此页面。</AlertBody>
        </Alert>
      )}

      {state === "polling" && (
        <Alert $type="info" className="mb-4">
          <AlertTitle>等待路由器重新上线…</AlertTitle>
          <AlertBody>
            <p>路由器正在重启网络服务，自动检测连接中，请稍候。</p>
            {newUrl && (
              <p className="mt-2 font-medium">
                如长时间无法连接，请手动访问：
                <a href={newUrl} className="underline ml-1">
                  {newUrl}
                </a>
              </p>
            )}
          </AlertBody>
        </Alert>
      )}

      {state === "success" && (
        <Alert $type="success" className="mb-4">
          <AlertTitle>配置已应用</AlertTitle>
          <AlertBody>路由器已成功上线，正在跳转到管理界面…</AlertBody>
        </Alert>
      )}

      {state === "error" && (
        <Alert $type="error" className="mb-4">
          <AlertTitle>应用失败</AlertTitle>
          <AlertBody>{errorMsg}</AlertBody>
        </Alert>
      )}

      <FooterRow>
        <Button $variant="secondary" onClick={onBack} disabled={state === "applying" || state === "polling"}>
          上一步
        </Button>
        {(state === "idle" || state === "error") && (
          <Button onClick={() => void handleApply()}>应用配置</Button>
        )}
      </FooterRow>
    </div>
  );
}
