import { TanStackDevtools } from "@tanstack/react-devtools";
import { formDevtoolsPlugin } from "@tanstack/react-form-devtools";
import { pacerDevtoolsPlugin } from "@tanstack/react-pacer-devtools";
import { ReactQueryDevtoolsPanel } from "@tanstack/react-query-devtools";

// DEV ONLY. Loaded via React.lazy behind `import.meta.env.DEV` in Providers.tsx
// so none of this reaches the production bundle (see AGENTS.md).
export default function DevtoolsPanel() {
  return (
    <TanStackDevtools
      plugins={[
        formDevtoolsPlugin(),
        { name: "TanStack Query", render: <ReactQueryDevtoolsPanel /> },
        pacerDevtoolsPlugin(),
      ]}
    />
  );
}
