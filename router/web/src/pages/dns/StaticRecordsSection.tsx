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
  Input,
} from "../../components/ui/index.ts";
import { fieldErrorText, staticRecordsFormSchema, type StaticRecordsFormValues } from "../../lib/formSchemas.ts";

const RecordRow = twc.div`flex items-start gap-2 mb-2`;
const FieldCol = twc.div`flex-1`;
const FieldError = twc.p`text-xs text-red-600 mt-1`;
const InvalidInput = twc(Input)`border-red-400 focus:ring-red-500`;
const RemoveButton = twc.button`
  shrink-0 h-9 w-9 mt-0.5 flex items-center justify-center rounded-md
  text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors
`;
const ColLabel = twc.span`block text-xs text-gray-500 mb-1`;
const EmptyHint = twc.p`text-sm text-gray-500 italic mb-3`;
const FooterRow = twc.div`flex items-center justify-between w-full`;
const CheckLabel = twc.label`flex items-center gap-2 text-sm text-gray-700 cursor-pointer`;

interface Props {
  config: RouterConfig;
}

export function StaticRecordsSection({ config }: Props) {
  const latestConfigQuery = useConfigQuery();
  const saveMutation = useSaveConfigMutation();
  const [saveError, setSaveError] = useState("");
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyUnchecked, setApplyUnchecked] = useState(false);

  const form = useForm({
    defaultValues: {
      records: config.dns.static_records,
    } as StaticRecordsFormValues,
    validators: { onChange: staticRecordsFormSchema, onSubmit: staticRecordsFormSchema },
    onSubmit: async ({ value }) => {
      setSaveError("");
      const latest = latestConfigQuery.data ?? config;
      const next: RouterConfig = {
        ...latest,
        dns: { ...latest.dns, static_records: value.records },
      };
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
      <form
        className="mt-6"
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
            <CardTitle>静态 DNS 记录</CardTitle>
          </CardHeader>
          <CardBody>
            <form.Field name="records" mode="array">
              {(listField) => (
                <>
                  {listField.state.value.length === 0 && (
                    <EmptyHint>暂无记录，点击下方按钮添加</EmptyHint>
                  )}
                  {listField.state.value.map((_, idx) => (
                    <RecordRow key={idx}>
                      <form.Field name={`records[${idx}].name`}>
                        {(nameField) => {
                          const err = fieldErrorText(nameField.state.meta.errors);
                          const Inp = err ? InvalidInput : Input;
                          return (
                            <FieldCol>
                              <ColLabel>域名</ColLabel>
                              <Inp
                                value={nameField.state.value}
                                onChange={(e) => nameField.handleChange(e.target.value)}
                                onBlur={nameField.handleBlur}
                                placeholder="nas.lan"
                              />
                              {err && <FieldError>{err}</FieldError>}
                            </FieldCol>
                          );
                        }}
                      </form.Field>
                      <form.Field name={`records[${idx}].ip`}>
                        {(ipField) => {
                          const err = fieldErrorText(ipField.state.meta.errors);
                          const Inp = err ? InvalidInput : Input;
                          return (
                            <FieldCol>
                              <ColLabel>IP 地址</ColLabel>
                              <Inp
                                value={ipField.state.value}
                                onChange={(e) => ipField.handleChange(e.target.value)}
                                onBlur={ipField.handleBlur}
                                placeholder="192.168.20.10"
                              />
                              {err && <FieldError>{err}</FieldError>}
                            </FieldCol>
                          );
                        }}
                      </form.Field>
                      <RemoveButton
                        type="button"
                        aria-label={`删除第 ${idx + 1} 条记录`}
                        onClick={() => listField.removeValue(idx)}
                      >
                        ✕
                      </RemoveButton>
                    </RecordRow>
                  ))}
                  <Button
                    type="button"
                    $variant="secondary"
                    $size="sm"
                    className="mt-1"
                    onClick={() => listField.pushValue({ name: "", ip: "" })}
                  >
                    + 添加记录
                  </Button>
                </>
              )}
            </form.Field>
          </CardBody>
          <CardFooter>
            <FooterRow>
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

      <ApplyDialog
        open={applyOpen}
        unchecked={applyUnchecked}
        onClose={() => setApplyOpen(false)}
        onSuccess={() => setApplyOpen(false)}
      />
    </>
  );
}
