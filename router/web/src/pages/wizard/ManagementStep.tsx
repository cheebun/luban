import { useForm } from "@tanstack/react-form";
import { twc } from "react-twc";
import { Button, FormGroup, Input, Label, PasswordInput } from "../../components/ui/index.ts";
import { fieldErrorText, wizardManagementFormSchema } from "../../lib/formSchemas.ts";
import type { WizardFormData } from "./WizardPage.tsx";

const FooterRow = twc.div`flex items-center justify-between mt-6`;
const FieldError = twc.p`text-xs text-red-600 mt-1`;
const InlineRow = twc.div`flex items-start gap-3`;
const InlineItem = twc.div`flex-1`;

interface Props {
  formData: WizardFormData;
  onUpdate: (updates: Partial<WizardFormData>) => void;
  onNext: () => void;
  onBack: () => void;
}

export function ManagementStep({ formData, onUpdate, onNext, onBack }: Props) {
  const form = useForm({
    defaultValues: {
      lanIp: formData.lanIp,
      lanMask: formData.lanMask,
      password: formData.password,
      confirmPassword: "",
    },
    validators: { onChange: wizardManagementFormSchema, onSubmit: wizardManagementFormSchema },
    onSubmit: ({ value }) => {
      onUpdate({ lanIp: value.lanIp, lanMask: value.lanMask, password: value.password });
      onNext();
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void form.handleSubmit();
      }}
    >
      <FormGroup>
        <Label>LAN 地址</Label>
        <InlineRow>
          <InlineItem>
            <form.Field name="lanIp">
              {(field) => (
                <>
                  <Input
                    id="lan-ip"
                    placeholder="192.168.20.1"
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                  />
                  {fieldErrorText(field.state.meta.errors) && (
                    <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                  )}
                </>
              )}
            </form.Field>
          </InlineItem>
          <span className="text-gray-500 mt-2.5">/</span>
          <InlineItem>
            <form.Field name="lanMask">
              {(field) => (
                <>
                  <Input
                    id="lan-mask"
                    placeholder="255.255.255.0"
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                  />
                  {fieldErrorText(field.state.meta.errors) && (
                    <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                  )}
                </>
              )}
            </form.Field>
          </InlineItem>
        </InlineRow>
        <p className="text-xs text-gray-500 mt-1">
          路由器管理界面将在此地址上可访问，默认 192.168.20.1 / 255.255.255.0
        </p>
      </FormGroup>

      <FormGroup>
        <Label htmlFor="mgmt-password">新管理员密码</Label>
        <form.Field name="password">
          {(field) => (
            <>
              <PasswordInput
                id="mgmt-password"
                placeholder="至少 8 位"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
                autoComplete="new-password"
              />
              {fieldErrorText(field.state.meta.errors) && (
                <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
              )}
            </>
          )}
        </form.Field>
      </FormGroup>

      <FormGroup>
        <Label htmlFor="mgmt-confirm">确认密码</Label>
        <form.Field name="confirmPassword">
          {(field) => (
            <>
              <PasswordInput
                id="mgmt-confirm"
                placeholder="再次输入密码"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
                autoComplete="new-password"
              />
              {fieldErrorText(field.state.meta.errors) && (
                <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
              )}
            </>
          )}
        </form.Field>
      </FormGroup>

      <FooterRow>
        <Button $variant="secondary" type="button" onClick={onBack}>
          上一步
        </Button>
        <form.Subscribe selector={(s) => s.isSubmitting}>
          {(isSubmitting) => (
            <Button type="submit" disabled={isSubmitting}>
              下一步
            </Button>
          )}
        </form.Subscribe>
      </FooterRow>
    </form>
  );
}
