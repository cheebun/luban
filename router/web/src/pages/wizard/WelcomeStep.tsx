import { twc } from "react-twc";
import { Button } from "../../components/ui/index.ts";

const Title = twc.h2`text-xl font-semibold text-gray-900 mb-3`;
const Body = twc.p`text-sm text-gray-600 mb-3`;
const FeatureList = twc.ul`list-disc list-inside text-sm text-gray-600 space-y-1 mb-6`;

interface Props {
  onNext: () => void;
}

export function WelcomeStep({ onNext }: Props) {
  return (
    <div>
      <Title>欢迎使用鲁班路由器</Title>
      <Body>
        这是鲁班路由器的首次启动向导。向导将帮助您完成以下配置，整个过程大约需要 3–5 分钟：
      </Body>
      <FeatureList>
        <li>识别网络接口硬件及板卡型号</li>
        <li>分配 WAN（上行）和 LAN（内网）端口</li>
        <li>设置内网地址与管理员密码</li>
        <li>一键应用配置，路由器立即上线</li>
      </FeatureList>
      <Body>完成向导后，路由器将重启网络服务并自动跳转到管理页面。</Body>
      <div className="flex justify-end">
        <Button onClick={onNext}>开始配置</Button>
      </div>
    </div>
  );
}
