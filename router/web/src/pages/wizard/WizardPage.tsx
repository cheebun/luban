import { useEffect, useState } from "react";
import { twc } from "react-twc";
import { getWizardState } from "../../api/index.ts";
import type { WizardInterface, WizardState } from "../../api/index.ts";
import { Alert, AlertBody, Stepper } from "../../components/ui/index.ts";
import { ApplyStep } from "./ApplyStep.tsx";
import { HardwareStep } from "./HardwareStep.tsx";
import { ManagementStep } from "./ManagementStep.tsx";
import { PortStep } from "./PortStep.tsx";
import { WelcomeStep } from "./WelcomeStep.tsx";

// Exported so step components can import it as a shared type without duplication.
export interface WizardFormData {
  wanInterface: string;
  lanInterfaces: string[];
  lanIp: string;
  lanMask: string;
  password: string;
}

const DEFAULT_FORM: WizardFormData = {
  wanInterface: "",
  lanInterfaces: [],
  lanIp: "192.168.20.1",
  lanMask: "255.255.255.0",
  password: "",
};

const STEP_LABELS = ["欢迎", "硬件识别", "端口分配", "管理设置", "应用"];

const Shell = twc.div`min-h-screen bg-gray-50 flex items-center justify-center p-4`;
const Panel = twc.div`w-full max-w-2xl bg-white rounded-xl shadow-sm border border-gray-200 p-8`;
const PanelTitle = twc.h1`text-2xl font-bold text-gray-900 mb-6`;
const StepTitle = twc.h2`text-base font-semibold text-gray-700 mb-4`;

const STEP_TITLES = ["欢迎", "硬件识别", "端口分配", "管理设置", "确认应用"];

// Derive initial port suggestions from wizard state interface roles.
function prefillPorts(interfaces: WizardInterface[]): Partial<WizardFormData> {
  const wan = interfaces.find((i) => i.role_suggestion === "wan");
  const lans = interfaces.filter((i) => i.role_suggestion === "lan");
  return {
    wanInterface: wan?.name ?? "",
    lanInterfaces: lans.map((i) => i.name),
  };
}

export function WizardPage() {
  const [step, setStep] = useState(0);
  const [formData, setFormData] = useState<WizardFormData>(DEFAULT_FORM);
  const [wizardState, setWizardState] = useState<WizardState | null>(null);
  const [loadError, setLoadError] = useState("");

  useEffect(() => {
    getWizardState()
      .then((s) => {
        setWizardState(s);
        // Pre-fill ports from role suggestions on initial load.
        setFormData((prev) => ({ ...prev, ...prefillPorts(s.interfaces) }));
      })
      .catch(() => setLoadError("无法加载向导状态，请刷新页面重试。"));
  }, []);

  function updateForm(updates: Partial<WizardFormData>) {
    setFormData((prev) => ({ ...prev, ...updates }));
  }

  function updateWizardState(updater: (prev: WizardState) => WizardState) {
    setWizardState((prev) => (prev ? updater(prev) : prev));
  }

  function goToPortStep() {
    // Re-apply role suggestions based on the (possibly probed) interface list.
    if (wizardState) {
      setFormData((prev) => ({ ...prev, ...prefillPorts(wizardState.interfaces) }));
    }
    setStep(2);
  }

  return (
    <Shell>
      <Panel>
        <PanelTitle>鲁班初始化向导</PanelTitle>
        <Stepper steps={STEP_LABELS} current={step} />

        {loadError && (
          <Alert $type="error" className="mb-4">
            <AlertBody>{loadError}</AlertBody>
          </Alert>
        )}

        <StepTitle>{STEP_TITLES[step]}</StepTitle>

        {step === 0 && <WelcomeStep onNext={() => setStep(1)} />}

        {step === 1 &&
          (wizardState ? (
            <HardwareStep
              state={wizardState}
              onStateUpdate={updateWizardState}
              onNext={goToPortStep}
              onBack={() => setStep(0)}
            />
          ) : !loadError ? (
            <p className="text-sm text-gray-400">正在加载硬件信息…</p>
          ) : null)}

        {step === 2 && (
          <PortStep
            interfaces={wizardState?.interfaces ?? []}
            formData={formData}
            onUpdate={updateForm}
            onNext={() => setStep(3)}
            onBack={() => setStep(1)}
          />
        )}

        {step === 3 && (
          <ManagementStep
            formData={formData}
            onUpdate={updateForm}
            onNext={() => setStep(4)}
            onBack={() => setStep(2)}
          />
        )}

        {step === 4 && (
          <ApplyStep
            formData={formData}
            interfaces={wizardState?.interfaces ?? []}
            onBack={() => setStep(3)}
          />
        )}
      </Panel>
    </Shell>
  );
}
