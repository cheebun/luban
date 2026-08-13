import { twc } from "react-twc";
import { Input } from "./Input.tsx";

const UnitInputField = twc(Input)`pr-12`;

const UnitWrapper = twc.div`relative`;

const UnitLabel = twc.span`pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-400`;

export type UnitInputProps = React.ComponentProps<"input"> & {
  unit: string;
};

export function UnitInput({ unit, ...props }: UnitInputProps) {
  return (
    <UnitWrapper>
      <UnitInputField {...props} />
      <UnitLabel>{unit}</UnitLabel>
    </UnitWrapper>
  );
}
