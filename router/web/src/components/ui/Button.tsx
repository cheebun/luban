import { tv, type VariantProps } from "tailwind-variants";
import { twc } from "react-twc";

const buttonVariants = tv({
  base: "inline-flex items-center justify-center rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 cursor-pointer",
  variants: {
    $variant: {
      primary: "bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-500",
      secondary: "bg-gray-100 text-gray-900 hover:bg-gray-200 focus-visible:ring-gray-400",
      danger: "bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500",
      ghost: "text-gray-700 hover:bg-gray-100 focus-visible:ring-gray-400",
    },
    $size: {
      sm: "h-8 px-3 text-sm gap-1.5",
      md: "h-10 px-4 text-sm gap-2",
      lg: "h-11 px-6 text-base gap-2",
    },
  },
  defaultVariants: {
    $variant: "primary",
    $size: "md",
  },
});

export type ButtonProps = React.ComponentProps<"button"> & VariantProps<typeof buttonVariants>;

export const Button = twc.button<ButtonProps>((props) => buttonVariants(props));
