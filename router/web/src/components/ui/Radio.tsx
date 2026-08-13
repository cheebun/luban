import { twc } from "react-twc";

export const RadioGroup = twc.div`flex items-center gap-6`;

const RadioOptionLabel = twc.label`flex items-center gap-2 text-sm text-gray-700 cursor-pointer`;

const RadioInputEl = twc.input`h-4 w-4 border-gray-300 text-blue-600 focus:ring-blue-500 cursor-pointer`;

export interface RadioOptionProps<T extends string> {
  name: string;
  value: T;
  checked: boolean;
  onChange: (value: T) => void;
  children: React.ReactNode;
  id?: string;
}

export function RadioOption<T extends string>({
  name,
  value,
  checked,
  onChange,
  children,
  id,
}: RadioOptionProps<T>) {
  return (
    <RadioOptionLabel htmlFor={id}>
      <RadioInputEl
        type="radio"
        id={id}
        name={name}
        value={value}
        checked={checked}
        onChange={() => onChange(value)}
      />
      {children}
    </RadioOptionLabel>
  );
}
