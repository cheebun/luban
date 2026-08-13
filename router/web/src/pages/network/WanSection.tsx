import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import type { RouterConfig, WanMode } from "../../api/index.ts";
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
  FormGroup,
  Label,
  PasswordInput,
  Select,
} from "../../components/ui/index.ts";
import { fieldErrorText, wanFormSchema, type WanFormValues } from "../../lib/formSchemas.ts";
import { maskToPrefix, splitCidrForEdit } from "../../lib/network.ts";
import { CheckLabel, DhcpAutoNote, FieldError, FooterRow } from "./primitives.tsx";
import { TextField } from "./TextField.tsx";

interface Props {
  config: RouterConfig;
  onApplied: (unchecked: boolean) => void;
}

function wanDefaults(config: RouterConfig): WanFormValues {
  const { ip, mask } = splitCidrForEdit(config.wan.static.address);
  return {
    mode: config.wan.mode,
    ip,
    mask,
    gateway: config.wan.static.gateway,
    dns1: config.wan.static.dns[0] ?? "",
    dns2: config.wan.static.dns[1] ?? "",
    username: config.wan.pppoe.username,
    password: config.wan.pppoe.password,
  };
}

// WAN section — connection mode + static/PPPoE credentials. `config` is the
// snapshot the form was seeded from at mount (NetworkPage only renders
// sections once the initial config load resolves, so this is real data, not
// a placeholder); submits always merge onto the latest cache value via
// useConfigQuery() so concurrent edits in other sections aren't clobbered.
export function WanSection({ config, onApplied }: Props) {
  const latestConfigQuery = useConfigQuery();
  const saveMutation = useSaveConfigMutation();
  const [unchecked, setUnchecked] = useState(false);
  const [saveError, setSaveError] = useState("");

  const form = useForm({
    defaultValues: wanDefaults(config),
    validators: { onChange: wanFormSchema, onSubmit: wanFormSchema },
    onSubmit: async ({ value }) => {
      setSaveError("");
      const latest = latestConfigQuery.data ?? config;
      const next: RouterConfig = {
        ...latest,
        wan: {
          ...latest.wan,
          mode: value.mode,
          static:
            value.mode === "static"
              ? {
                  ...latest.wan.static,
                  address: `${value.ip}/${maskToPrefix(value.mask)}`,
                  gateway: value.gateway,
                  dns: [value.dns1, value.dns2].filter((d) => d !== ""),
                }
              : latest.wan.static,
          pppoe:
            value.mode === "pppoe"
              ? { ...latest.wan.pppoe, username: value.username, password: value.password }
              : latest.wan.pppoe,
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
          <CardTitle>联网设置（WAN）</CardTitle>
        </CardHeader>
        <CardBody>
          {saveError && (
            <Alert $type="error" className="mb-4">
              <AlertBody>{saveError}</AlertBody>
            </Alert>
          )}

          <form.Field name="mode">
            {(field) => (
              <FormGroup>
                <Label htmlFor="wan-mode">连接模式</Label>
                <Select
                  id="wan-mode"
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value as WanMode)}
                >
                  <option value="dhcp">动态 IP</option>
                  <option value="pppoe">PPPoE</option>
                  <option value="static">静态 IP</option>
                  <option value="bridge">有线桥</option>
                </Select>
              </FormGroup>
            )}
          </form.Field>

          <form.Subscribe selector={(s) => s.values.mode}>
            {(mode) => (
              <>
                {mode === "dhcp" && <DhcpAutoNote>从上级路由/光猫自动获取</DhcpAutoNote>}

                {mode === "bridge" && (
                  <Alert $type="warning" className="mb-4">
                    <AlertBody>
                      切换到有线桥后，本设备退化为交换机，DHCP/NAT
                      由上级路由负责；管理地址将由上级路由分配，应用后需通过上级路由的客户端列表查找本机新地址访问。90
                      秒内无法确认将自动回滚。
                    </AlertBody>
                  </Alert>
                )}

                {mode === "pppoe" && (
                  <>
                    <form.Field name="username">
                      {(field) => (
                        <TextField field={field} id="pppoe-user" label="用户名 *" autoComplete="username" />
                      )}
                    </form.Field>
                    <form.Field name="password">
                      {(field) => (
                        <FormGroup>
                          <Label htmlFor="pppoe-pass">密码 *</Label>
                          <PasswordInput
                            id="pppoe-pass"
                            autoComplete="current-password"
                            value={field.state.value}
                            onChange={(e) => field.handleChange(e.target.value)}
                            onBlur={field.handleBlur}
                          />
                          {fieldErrorText(field.state.meta.errors) && (
                            <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                          )}
                        </FormGroup>
                      )}
                    </form.Field>
                  </>
                )}

                {mode === "static" && (
                  <>
                    <form.Field name="ip">
                      {(field) => (
                        <TextField field={field} id="wan-ip" label="IP 地址 *" placeholder="例：203.0.113.2" />
                      )}
                    </form.Field>
                    <form.Field name="mask">
                      {(field) => (
                        <TextField
                          field={field}
                          id="wan-mask"
                          label="子网掩码 *"
                          placeholder="例：255.255.255.0"
                        />
                      )}
                    </form.Field>
                    <form.Field name="gateway">
                      {(field) => (
                        <TextField
                          field={field}
                          id="wan-gateway"
                          label="默认网关 *"
                          placeholder="例：203.0.113.1"
                        />
                      )}
                    </form.Field>
                    <form.Field name="dns1">
                      {(field) => (
                        <TextField field={field} id="wan-dns1" label="首选 DNS *" placeholder="例：223.5.5.5" />
                      )}
                    </form.Field>
                    <form.Field name="dns2">
                      {(field) => (
                        <TextField
                          field={field}
                          id="wan-dns2"
                          label="备用 DNS"
                          placeholder="例：114.114.114.114（可选）"
                        />
                      )}
                    </form.Field>
                  </>
                )}
              </>
            )}
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
