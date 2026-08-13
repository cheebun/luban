// Layout/typography primitives shared by the WAN/LAN/MTU section components.
// Same "compound primitive kit" exemption as components/ui/Card.tsx — see
// AGENTS.md's one-component-per-file rule.
import { twc } from "react-twc";
import { Input, UnitInput } from "../../components/ui/index.ts";

export const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;

export const SectionStack = twc.div`flex flex-col gap-4 max-w-2xl`;

export const InlineRow = twc.div`flex items-center gap-3`;

export const InlineRowItem = twc.div`flex-1`;

export const CheckLabel = twc.label`flex items-center gap-2 text-sm text-gray-700 cursor-pointer`;

export const FooterRow = twc.div`flex items-center justify-between w-full`;

export const InvalidInput = twc(Input)`border-red-400 focus:ring-red-500`;

export const InvalidUnitInput = twc(UnitInput)`[&_input]:border-red-400 [&_input]:focus:ring-red-500`;

export const FieldError = twc.p`text-xs text-red-600 mt-1`;

export const DhcpAutoNote = twc.p`text-sm text-gray-500`;
