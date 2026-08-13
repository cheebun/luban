import type { AnyFieldApi } from "@tanstack/react-form";
import { FormGroup, Input, Label } from "../../components/ui/index.ts";
import { fieldErrorText } from "../../lib/formSchemas.ts";
import { FieldError, InvalidInput } from "./primitives.tsx";

type InputProps = React.ComponentProps<typeof Input>;

interface Props extends Omit<InputProps, "value" | "onChange" | "onBlur"> {
  field: AnyFieldApi;
  label?: string;
}

// Shared string-input <-> TanStack Form field binding used by WanSection,
// LanSection, and DnsPage: swaps Input -> InvalidInput on error and renders
// the field's message via the shared FieldError styling. Only for plain
// string fields — number/unit fields (lease hours, MTU/MSS) keep their own
// UnitInput wiring since the value coercion differs.
export function TextField({ field, label, id, ...inputProps }: Props) {
  const error = fieldErrorText(field.state.meta.errors);
  const InputField = error ? InvalidInput : Input;
  const input = (
    <InputField
      id={id}
      value={field.state.value as string}
      onChange={(e) => field.handleChange(e.target.value)}
      onBlur={field.handleBlur}
      {...inputProps}
    />
  );
  if (!label) {
    return (
      <>
        {input}
        {error && <FieldError>{error}</FieldError>}
      </>
    );
  }
  return (
    <FormGroup>
      <Label htmlFor={id}>{label}</Label>
      {input}
      {error && <FieldError>{error}</FieldError>}
    </FormGroup>
  );
}
