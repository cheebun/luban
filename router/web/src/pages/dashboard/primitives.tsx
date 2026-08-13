// Layout/typography primitives shared by the dashboard card components.
// Same "compound primitive kit" exemption as components/ui/Card.tsx — these
// are single-tag styling wrappers with no independent logic, not page-level
// components, so grouping them in one file doesn't violate the
// one-component-per-file rule (see AGENTS.md).
import { twc } from "react-twc";

export const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;

export const GridLayout = twc.div`grid gap-4 md:grid-cols-2 xl:grid-cols-3`;

export const StatLabel = twc.dt`text-xs font-medium text-gray-500 uppercase tracking-wide`;

export const StatValue = twc.dd`mt-1 text-sm font-semibold text-gray-900`;

export const DlGrid = twc.dl`grid grid-cols-2 gap-x-4 gap-y-3`;

export const TableWrapper = twc.div`overflow-x-auto`;

export const Table = twc.table`min-w-full text-sm`;

export const Thead = twc.thead`border-b border-gray-200`;

export const Th = twc.th`text-left text-xs font-medium text-gray-500 uppercase tracking-wide pb-2 pr-4`;

export const Td = twc.td`py-2 pr-4 text-gray-700`;

export const RefreshRow = twc.div`flex items-center justify-between mb-6`;

export const RefreshNote = twc.span`text-xs text-gray-400`;

export const ChipRow = twc.div`flex flex-wrap gap-2`;

export const Chip = twc.span`inline-flex items-center rounded-md bg-gray-100 px-2 py-1 text-xs font-mono text-gray-700`;

export const MutedNote = twc.p`text-sm text-gray-400`;
