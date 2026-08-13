import { NavLink } from "react-router-dom";
import { tv } from "tailwind-variants";
import { twc } from "react-twc";

const navItemVariants = tv({
  base: "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
  variants: {
    $active: {
      true: "bg-blue-50 text-blue-700",
      false: "text-gray-600 hover:bg-gray-100 hover:text-gray-900",
    },
  },
  defaultVariants: {
    $active: false,
  },
});

interface NavItemWrapperProps extends React.ComponentProps<"a"> {
  $active?: boolean;
}

const NavItemWrapper = twc.a<NavItemWrapperProps>((props) => navItemVariants(props));

interface NavItem {
  to: string;
  label: string;
  icon: string;
}

const NAV_ITEMS: NavItem[] = [
  { to: "/", label: "概述", icon: "◉" },
  { to: "/network", label: "网络设置", icon: "⚙" },
  { to: "/dns", label: "DNS 设置", icon: "◎" },
  { to: "/health", label: "系统自检", icon: "✚" },
  { to: "/log", label: "系统日志", icon: "≡" },
];

export const NavIconText = twc.span`text-base leading-none`;

export const NavLabelText = twc.span``;

export function Nav() {
  return (
    <nav aria-label="主导航">
      <ul role="list" className="space-y-0.5">
        {NAV_ITEMS.map((item) => (
          <li key={item.to}>
            <NavLink to={item.to} end={item.to === "/"}>
              {({ isActive }) => (
                <NavItemWrapper $active={isActive}>
                  <NavIconText>{item.icon}</NavIconText>
                  <NavLabelText>{item.label}</NavLabelText>
                </NavItemWrapper>
              )}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
