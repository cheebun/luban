import { tv, type VariantProps } from "tailwind-variants";
import { twc } from "react-twc";

const badgeVariants = tv({
  base: "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium",
  variants: {
    $status: {
      online: "bg-green-100 text-green-800",
      offline: "bg-gray-100 text-gray-600",
      warning: "bg-amber-100 text-amber-800",
      error: "bg-red-100 text-red-700",
    },
  },
  defaultVariants: {
    $status: "offline",
  },
});

export type BadgeProps = React.ComponentProps<"span"> & VariantProps<typeof badgeVariants>;

export const Badge = twc.span<BadgeProps>((props) => badgeVariants(props));
