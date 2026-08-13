import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { twc } from "react-twc";
import { ApiRequestError } from "../api/index.ts";
import { useChangePasswordMutation, useLoginMutation } from "../api/mutations.ts";
import {
  Alert,
  AlertBody,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  FormGroup,
  Input,
  Label,
} from "../components/ui/index.ts";
import { changePasswordFormSchema, fieldErrorText, loginFormSchema } from "../lib/formSchemas.ts";
import { useAuth } from "../store/authStore.ts";

const PageCenter = twc.div`
  min-h-screen bg-gray-50 flex items-center justify-center p-4
`;

const LoginCard = twc(Card)`w-full max-w-sm`;

const LogoRow = twc.div`flex items-baseline gap-2 mb-1`;

const LogoText = twc.span`text-2xl font-bold text-gray-900`;

const LogoSub = twc.span`text-sm text-gray-400`;

const Subtitle = twc.p`text-sm text-gray-500`;

const FieldError = twc.p`text-xs text-red-600 mt-1`;

type Mode = "login" | "change_password";

export function LoginPage() {
  const navigate = useNavigate();
  const { setAuth } = useAuth();
  const [mode, setMode] = useState<Mode>("login");
  const [error, setError] = useState("");
  // Kept outside the login form so the change-password step can send it back
  // as `current` without asking the user to re-type it.
  const [currentPassword, setCurrentPassword] = useState("");

  const loginMutation = useLoginMutation();
  const changePasswordMutation = useChangePasswordMutation();

  const loginForm = useForm({
    defaultValues: { username: "admin", password: "" },
    validators: { onSubmit: loginFormSchema },
    onSubmit: async ({ value }) => {
      setError("");
      try {
        const res = await loginMutation.mutateAsync(value);
        if (res.must_change) {
          setCurrentPassword(value.password);
          setMode("change_password");
        } else {
          setAuth(true);
          navigate("/", { replace: true });
        }
      } catch (err) {
        if (err instanceof ApiRequestError && err.status === 401) {
          setError("用户名或密码错误");
        } else {
          setError("登录失败，请重试");
        }
      }
    },
  });

  const changePasswordForm = useForm({
    defaultValues: { newPassword: "", confirmPassword: "" },
    validators: { onSubmit: changePasswordFormSchema },
    onSubmit: async ({ value }) => {
      setError("");
      try {
        await changePasswordMutation.mutateAsync({
          current: currentPassword,
          new: value.newPassword,
        });
        setAuth(true);
        navigate("/", { replace: true });
      } catch {
        setError("修改密码失败，请重试");
      }
    },
  });

  if (mode === "change_password") {
    return (
      <PageCenter>
        <LoginCard>
          <CardHeader>
            <LogoRow>
              <LogoText>鲁班</LogoText>
              <LogoSub>Luban</LogoSub>
            </LogoRow>
            <CardTitle>修改初始密码</CardTitle>
            <Subtitle>首次登录需要修改密码</Subtitle>
          </CardHeader>
          <CardBody>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void changePasswordForm.handleSubmit();
              }}
            >
              {error && (
                <Alert $type="error" className="mb-4">
                  <AlertBody>{error}</AlertBody>
                </Alert>
              )}
              <changePasswordForm.Field name="newPassword">
                {(field) => (
                  <FormGroup>
                    <Label htmlFor="new-password">新密码</Label>
                    <Input
                      id="new-password"
                      type="password"
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="至少 8 位"
                      required
                    />
                    {fieldErrorText(field.state.meta.errors) && (
                      <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                    )}
                  </FormGroup>
                )}
              </changePasswordForm.Field>
              <changePasswordForm.Field name="confirmPassword">
                {(field) => (
                  <FormGroup>
                    <Label htmlFor="confirm-password">确认新密码</Label>
                    <Input
                      id="confirm-password"
                      type="password"
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="再次输入新密码"
                      required
                    />
                    {fieldErrorText(field.state.meta.errors) && (
                      <FieldError>{fieldErrorText(field.state.meta.errors)}</FieldError>
                    )}
                  </FormGroup>
                )}
              </changePasswordForm.Field>
              <changePasswordForm.Subscribe selector={(s) => s.isSubmitting}>
                {(isSubmitting) => (
                  <Button type="submit" className="w-full" disabled={isSubmitting}>
                    {isSubmitting ? "正在修改…" : "确认修改"}
                  </Button>
                )}
              </changePasswordForm.Subscribe>
            </form>
          </CardBody>
        </LoginCard>
      </PageCenter>
    );
  }

  return (
    <PageCenter>
      <LoginCard>
        <CardHeader>
          <LogoRow>
            <LogoText>鲁班</LogoText>
            <LogoSub>Luban</LogoSub>
          </LogoRow>
          <CardTitle>路由器管理</CardTitle>
          <Subtitle>请登录以继续</Subtitle>
        </CardHeader>
        <CardBody>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void loginForm.handleSubmit();
            }}
          >
            {error && (
              <Alert $type="error" className="mb-4">
                <AlertBody>{error}</AlertBody>
              </Alert>
            )}
            <loginForm.Field name="username">
              {(field) => (
                <FormGroup>
                  <Label htmlFor="username">用户名</Label>
                  <Input
                    id="username"
                    type="text"
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    autoComplete="username"
                    required
                  />
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>{field.state.meta.errors[0]?.message}</FieldError>
                  )}
                </FormGroup>
              )}
            </loginForm.Field>
            <loginForm.Field name="password">
              {(field) => (
                <FormGroup>
                  <Label htmlFor="password">密码</Label>
                  <Input
                    id="password"
                    type="password"
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    autoComplete="current-password"
                    required
                  />
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>{field.state.meta.errors[0]?.message}</FieldError>
                  )}
                </FormGroup>
              )}
            </loginForm.Field>
            <loginForm.Subscribe selector={(s) => s.isSubmitting}>
              {(isSubmitting) => (
                <Button type="submit" className="w-full" disabled={isSubmitting}>
                  {isSubmitting ? "正在登录…" : "登录"}
                </Button>
              )}
            </loginForm.Subscribe>
          </form>
        </CardBody>
      </LoginCard>
    </PageCenter>
  );
}
