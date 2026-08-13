import { tv, type VariantProps } from "tailwind-variants";
import { twc } from "react-twc";

const alertVariants = tv({
  base: "rounded-md border px-4 py-3 text-sm",
  variants: {
    $type: {
      info: "border-blue-200 bg-blue-50 text-blue-900",
      success: "border-green-200 bg-green-50 text-green-900",
      warning: "border-amber-200 bg-amber-50 text-amber-900",
      error: "border-red-200 bg-red-50 text-red-900",
    },
  },
  defaultVariants: {
    $type: "info",
  },
});

export type AlertProps = React.ComponentProps<"div"> & VariantProps<typeof alertVariants>;

export const Alert = twc.div<AlertProps>((props) => alertVariants(props));

export const AlertTitle = twc.p`font-semibold mb-1`;

export const AlertBody = twc.p``;
