import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import type { LanDnsMode, RouterConfig } from "../../api/index.ts";
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
  RadioGroup,
  RadioOption,
  UnitInput,
} from "../../components/ui/index.ts";
import { fieldErrorText, lanFormSchema, type LanFormValues } from "../../lib/formSchemas.ts";
import { leaseToHours, maskToPrefix, splitCidrForEdit } from "../../lib/network.ts";
import { CheckLabel, FieldError, FooterRow, InlineRow, InlineRowItem, InvalidUnitInput } from "./primitives.tsx";
import { TextField } from "./TextField.tsx";

interface Props {
  config: RouterConfig;
  onApplied: (unchecked: boolean) => void;
}

function lanDefaults(config: RouterConfig): LanFormValues {
  const { ip, mask } = splitCidrForEdit(config.lan.address);
  return {
    ip,
    mask,
    dhcpEnabled: config.lan.dhcp.enabled,
    poolStart: config.lan.dhcp.start,
    poolEnd: config.lan.dhcp.end,
    leaseHours: leaseToHours(config.lan.dhcp.lease) ?? 12,
    dnsMode: config.lan.dhcp.dns_mode,
    dns1: config.lan.dhcp.dns_servers[0] ?? "",
    dns2: config.lan.dhcp.dns_servers[1] ?? "",
  };
}

// LAN section — subnet, DHCP pool/lease, LAN-side DNS mode. Same
// merge-onto-latest-config pattern as WanSection.
export function LanSection({ config, onApplied }: Props) {
  const latestConfigQuery = useConfigQuery();
  const saveMutation = useSaveConfigMutation();
  const [unchecked, setUnchecked] = useState(false);
  const [saveError, setSaveError] = useState("");

  const form = useForm({
    defaultValues: lanDefaults(config),
    validators: { onChange: lanFormSchema, onSubmit: lanFormSchema },
    onSubmit: async ({ value }) => {
      setSaveError("");
      const latest = latestConfigQuery.data ?? config;
      const prefix = maskToPrefix(value.mask)!;
      const next: RouterConfig = {
        ...latest,
        lan: {
          ...latest.lan,
          address: `${value.ip}/${prefix}`,
          dhcp: {
            ...latest.lan.dhcp,
            enabled: value.dhcpEnabled,
            start: value.poolStart,
            end: value.poolEnd,
            lease: `${value.leaseHours}h`,
            dns_mode: value.dnsMode,
            dns_servers:
              value.dnsMode === "manual" ? [value.dns1, value.dns2].filter((d) => d !== "") : [],
          },
        },
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
          <CardTitle>LAN 基本设置</CardTitle>
        </CardHeader>
        <CardBody>
          {saveError && (
            <Alert $type="error" className="mb-4">
              <AlertBody>{saveError}</AlertBody>
            </Alert>
          )}

          <form.Field name="ip">
            {(field) => <TextField field={field} id="lan-ip" label="IP 地址 *" />}
          </form.Field>
          <form.Field name="mask">
            {(field) => <TextField field={field} id="lan-mask" label="子网掩码 *" />}
          </form.Field>

          <form.Field name="dhcpEnabled">
            {(field) => (
              <FormGroup>
                <Label>DHCP 服务</Label>
                <RadioGroup>
                  <RadioOption
                    name="dhcp-enabled"
                    value="on"
                    checked={field.state.value}
                    onChange={() => field.handleChange(true)}
                  >
                    启用
                  </RadioOption>
                  <RadioOption
                    name="dhcp-enabled"
                    value="off"
                    checked={!field.state.value}
                    onChange={() => field.handleChange(false)}
                  >
                    关闭
                  </RadioOption>
                </RadioGroup>
              </FormGroup>
            )}
          </form.Field>

          <form.Subscribe selector={(s) => s.values.dhcpEnabled}>
            {(dhcpEnabled) =>
              dhcpEnabled && (
                <>
                  <FormGroup>
                    <Label>DHCP IP 池 *</Label>
                    <InlineRow>
                      <InlineRowItem>
                        <form.Field name="poolStart">
                          {(field) => <TextField field={field} aria-label="起始地址" placeholder="起始地址" />}
                        </form.Field>
                      </InlineRowItem>
                      <span>–</span>
                      <InlineRowItem>
                        <form.Field name="poolEnd">
                          {(field) => <TextField field={field} aria-label="结束地址" placeholder="结束地址" />}
                        </form.Field>
                      </InlineRowItem>
                    </InlineRow>
                  </FormGroup>
                  <form.Field name="leaseHours">
                    {(field) => (
                      <FormGroup>
                        <Label htmlFor="dhcp-lease">DHCP 租期 *</Label>
                        {(() => {
                          const error = fieldErrorText(field.state.meta.errors);
                          const Field = error ? InvalidUnitInput : UnitInput;
                          return (
                            <Field
                              id="dhcp-lease"
                              unit="小时"
                              type="number"
                              min={1}
                              value={field.state.value}
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
                      </FormGroup>
                    )}
                  </form.Field>
                </>
              )
            }
          </form.Subscribe>

          <form.Field name="dnsMode">
            {(field) => (
              <FormGroup>
                <Label>DNS 模式</Label>
                <RadioGroup>
                  <RadioOption
                    name="dns-mode"
                    value="auto"
                    checked={field.state.value === "auto"}
                    onChange={(v: LanDnsMode) => field.handleChange(v)}
                  >
                    自动
                  </RadioOption>
                  <RadioOption
                    name="dns-mode"
                    value="manual"
                    checked={field.state.value === "manual"}
                    onChange={(v: LanDnsMode) => field.handleChange(v)}
                  >
                    手动
                  </RadioOption>
                </RadioGroup>
                {field.state.value === "auto" && <FieldHint>下发路由器自身作为 DNS 服务器</FieldHint>}
              </FormGroup>
            )}
          </form.Field>

          <form.Subscribe selector={(s) => s.values.dnsMode}>
            {(dnsMode) =>
              dnsMode === "manual" && (
                <>
                  <form.Field name="dns1">
                    {(field) => (
                      <TextField field={field} id="lan-dns1" label="首选 DNS *" placeholder="例：223.5.5.5" />
                    )}
                  </form.Field>
                  <form.Field name="dns2">
                    {(field) => (
                      <TextField
                        field={field}
                        id="lan-dns2"
                        label="备用 DNS"
                        placeholder="例：114.114.114.114（可选）"
                      />
                    )}
                  </form.Field>
                </>
              )
            }
          </form.Subscribe>
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
