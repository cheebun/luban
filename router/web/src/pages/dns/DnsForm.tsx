import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { twc } from "react-twc";
import type { RouterConfig } from "../../api/index.ts";
import { useSaveConfigMutation } from "../../api/mutations.ts";
import { useConfigQuery } from "../../api/queries.ts";
import { ApplyDialog } from "../../components/apply/ApplyDialog.tsx";
import {
  Alert,
  AlertBody,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardTitle,
  FieldHint,
  FormGroup,
  Input,
  Label,
} from "../../components/ui/index.ts";
import { UPSTREAM_FORMATS_HINT, validateDnsUpstream } from "../../lib/dns.ts";
import { dnsFormSchema, fieldErrorText, type DnsFormValues } from "../../lib/formSchemas.ts";

const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;
const MaxWidthWrapper = twc.div`max-w-2xl`;
const ServerRow = twc.div`flex items-center gap-2 mb-2`;
const RemoveButton = twc.button`
  shrink-0 h-8 w-8 flex items-center justify-center rounded-md
  text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors
`;
const CheckLabel = twc.label`flex items-center gap-2 text-sm text-gray-700 cursor-pointer`;
const FooterRow = twc.div`flex items-center justify-between`;
const InvalidInput = twc(Input)`border-red-400 focus:ring-red-500`;
const FieldError = twc.p`text-xs text-red-600 mt-1`;

interface Props {
  config: RouterConfig;
}

// DNS upstream list — the canonical "optimistic saveConfig" case: this PUT
// hits the same /api/config endpoint and cache key as WAN/LAN/MTU, so
// useSaveConfigMutation's onMutate/onError/onSettled cache swap applies here
// unchanged (see api/mutations.ts). `config` is only used to seed the form
// once at mount; submits always merge onto useConfigQuery()'s latest data.
export function DnsForm({ config }: Props) {
  const latestConfigQuery = useConfigQuery();
  const saveMutation = useSaveConfigMutation();
  const [saveError, setSaveError] = useState("");
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyUnchecked, setApplyUnchecked] = useState(false);

  const form = useForm({
    defaultValues: { upstreams: config.dns.upstreams } as DnsFormValues,
    validators: { onChange: dnsFormSchema, onSubmit: dnsFormSchema },
    onSubmit: async ({ value }) => {
      setSaveError("");
      if (value.upstreams.some((s) => validateDnsUpstream(s) !== null)) {
        setSaveError("请修正上游 DNS 服务器格式后再提交");
        return;
      }
      const latest = latestConfigQuery.data ?? config;
      const next: RouterConfig = { ...latest, dns: { ...latest.dns, upstreams: value.upstreams } };
      try {
        await saveMutation.mutateAsync(next);
        setApplyOpen(true);
      } catch {
        setSaveError("保存失败，请重试");
      }
    },
  });

  return (
    <>
      <PageTitle>DNS 设置</PageTitle>
      <MaxWidthWrapper>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
        >
          {saveError && (
            <Alert $type="error" className="mb-4">
              <AlertBody>{saveError}</AlertBody>
            </Alert>
          )}

          <Card>
            <CardHeader>
              <CardTitle>SmartDNS 上游服务器</CardTitle>
            </CardHeader>
            <CardBody>
              <FormGroup>
                <Label>上游 DNS 服务器列表</Label>
                <form.Field name="upstreams" mode="array">
                  {(listField) => (
                    <>
                      {listField.state.value.map((server, idx) => (
                        <form.Field key={idx} name={`upstreams[${idx}]`}>
                          {(field) => {
                            const error =
                              fieldErrorText(field.state.meta.errors) ?? validateDnsUpstream(server);
                            const InputComponent = error ? InvalidInput : Input;
                            return (
                              <div>
                                <ServerRow>
                                  <InputComponent
                                    value={field.state.value}
                                    onChange={(e) => field.handleChange(e.target.value)}
                                    onBlur={field.handleBlur}
                                    placeholder="例：tls://223.5.5.5 或 114.114.114.114"
                                  />
                                  <RemoveButton
                                    type="button"
                                    aria-label={`删除第 ${idx + 1} 个服务器`}
                                    onClick={() => listField.removeValue(idx)}
                                  >
                                    ✕
                                  </RemoveButton>
                                </ServerRow>
                                {error && <FieldError>{error}</FieldError>}
                              </div>
                            );
                          }}
                        </form.Field>
                      ))}
                      <Button
                        type="button"
                        $variant="secondary"
                        $size="sm"
                        className="mt-2"
                        onClick={() => listField.pushValue("")}
                      >
                        + 添加服务器
                      </Button>
                    </>
                  )}
                </form.Field>
                <FieldHint>{UPSTREAM_FORMATS_HINT}</FieldHint>
              </FormGroup>
            </CardBody>
            <CardFooter>
              <FooterRow className="w-full">
                <CheckLabel>
                  <input
                    type="checkbox"
                    checked={applyUnchecked}
                    onChange={(e) => setApplyUnchecked(e.target.checked)}
                  />
                  不带回滚确认直接应用
                </CheckLabel>
                <form.Subscribe selector={(s) => s.isSubmitting}>
                  {(isSubmitting) => (
                    <Button type="submit" disabled={isSubmitting}>
                      保存并应用
                    </Button>
                  )}
                </form.Subscribe>
              </FooterRow>
            </CardFooter>
          </Card>
        </form>
      </MaxWidthWrapper>

      <ApplyDialog
        open={applyOpen}
        unchecked={applyUnchecked}
        onClose={() => setApplyOpen(false)}
        onSuccess={() => setApplyOpen(false)}
      />
    </>
  );
}
