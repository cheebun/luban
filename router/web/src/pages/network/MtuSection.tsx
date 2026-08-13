import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import type { RouterConfig } from "../../api/index.ts";
import { useSaveConfigMutation } from "../../api/mutations.ts";
import { useConfigQuery } from "../../api/queries.ts";
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
  Label,
  UnitInput,
} from "../../components/ui/index.ts";
import { buildMtuMssFormSchema, fieldErrorText, type MtuMssFormValues } from "../../lib/formSchemas.ts";
import { CheckLabel, FieldError, FooterRow, InvalidUnitInput } from "./primitives.tsx";

interface Props {
  config: RouterConfig;
  onApplied: (unchecked: boolean) => void;
}

// MTU/MSS section — hidden entirely in bridge mode by the NetworkPage
// orchestrator (no routed WAN link to tune). `effectiveMtu`/mss max both
// depend on `config.wan.mode`, which is why the validator is built per-render
// from the *saved* config rather than baked into a static schema.
export function MtuSection({ config, onApplied }: Props) {
  const latestConfigQuery = useConfigQuery();
  const saveMutation = useSaveConfigMutation();
  const [unchecked, setUnchecked] = useState(false);
  const [saveError, setSaveError] = useState("");

  const wanMode = (latestConfigQuery.data ?? config).wan.mode;
  const schema = buildMtuMssFormSchema(wanMode);

  const form = useForm({
    defaultValues: { mtu: config.wan.mtu, mss: config.wan.mss } as MtuMssFormValues,
    validators: { onChange: schema, onSubmit: schema },
    onSubmit: async ({ value }) => {
      setSaveError("");
      const latest = latestConfigQuery.data ?? config;
      const next: RouterConfig = {
        ...latest,
        wan: { ...latest.wan, mtu: value.mtu, mss: value.mss },
      };
      try {
        await saveMutation.mutateAsync(next);
        onApplied(unchecked);
      } catch {
        setSaveError("保存配置失败，请重试");
      }
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void form.handleSubmit();
      }}
    >
      <Card>
        <CardHeader>
          <CardTitle>MTU/MSS</CardTitle>
        </CardHeader>
        <CardBody>
          {saveError && (
            <Alert $type="error" className="mb-4">
              <AlertBody>{saveError}</AlertBody>
            </Alert>
          )}

          <form.Field name="mtu">
            {(field) => (
              <FormGroup>
                <Label htmlFor="wan-mtu">MTU</Label>
                {(() => {
                  const error = fieldErrorText(field.state.meta.errors);
                  const Field = error ? InvalidUnitInput : UnitInput;
                  return (
                    <Field
                      id="wan-mtu"
                      unit="字节"
                      type="number"
                      placeholder={wanMode === "pppoe" ? "自动：1492" : "自动：1500"}
                      value={field.state.value === 0 ? "" : field.state.value}
                      onChange={(e) =>
                        field.handleChange(e.target.value === "" ? 0 : Number(e.target.value))
                      }
                      onBlur={field.handleBlur}
                    />
                  );
                })()}
                {fieldErrorText(field.state.meta.errors) && (
                  <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                )}
                <FieldHint>留空为自动（PPPoE 1492 / 其他 1500）</FieldHint>
              </FormGroup>
            )}
          </form.Field>

          <form.Field name="mss">
            {(field) => (
              <FormGroup>
                <Label htmlFor="wan-mss">MSS</Label>
                {(() => {
                  const error = fieldErrorText(field.state.meta.errors);
                  const Field = error ? InvalidUnitInput : UnitInput;
                  return (
                    <Field
                      id="wan-mss"
                      unit="字节"
                      type="number"
                      placeholder="自动"
                      value={field.state.value === 0 ? "" : field.state.value}
                      onChange={(e) =>
                        field.handleChange(e.target.value === "" ? 0 : Number(e.target.value))
                      }
                      onBlur={field.handleBlur}
                    />
                  );
                })()}
                {fieldErrorText(field.state.meta.errors) && (
                  <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                )}
                <FieldHint>留空为自动（按当前 MTU 计算）</FieldHint>
              </FormGroup>
            )}
          </form.Field>
        </CardBody>
        <CardFooter>
          <FooterRow>
            <CheckLabel>
              <input
                type="checkbox"
                checked={unchecked}
                onChange={(e) => setUnchecked(e.target.checked)}
              />
              不带回滚确认直接应用
            </CheckLabel>
            <form.Subscribe selector={(s) => s.isSubmitting}>
              {(isSubmitting) => (
                <Button type="submit" disabled={isSubmitting}>
                  应用
                </Button>
              )}
            </form.Subscribe>
          </FooterRow>
        </CardFooter>
      </Card>
    </form>
  );
}
