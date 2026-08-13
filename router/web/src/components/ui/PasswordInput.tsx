import { useState } from "react";
import { twc } from "react-twc";
import { Input } from "./Input.tsx";

const Wrapper = twc.div`flex flex-col gap-1`;

const ToggleLabel = twc.label`flex items-center gap-1.5 text-xs text-gray-500 cursor-pointer select-none`;

const ToggleCheckbox = twc.input`h-3.5 w-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500`;

export type PasswordInputProps = Omit<React.ComponentProps<"input">, "type">;

export function PasswordInput(props: PasswordInputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <Wrapper>
      <Input type={visible ? "text" : "password"} {...props} />
      <ToggleLabel>
        <ToggleCheckbox
          type="checkbox"
          checked={visible}
          onChange={(e) => setVisible(e.target.checked)}
        />
        显示密码
      </ToggleLabel>
    </Wrapper>
  );
}
