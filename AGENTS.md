# AGENTS.md

Machine-readable rules for AI coding tools working on **Luban**, a
self-built Linux router (Debian/Armbian, Go backend `rui` + React web UI).
Code, comments, and commits are English only; the UI itself is Chinese.

For *why* behind any rule here, see [DECISIONS.md](DECISIONS.md) — this file
states the *what*, not the reasoning.

## Build / Test Commands

### `router/web/` (package `@router/web`)

```bash
pnpm install
pnpm --filter @router/web build    # tsc -b && vite build → dist/
pnpm --filter @router/web lint     # oxlint src/
pnpm --filter @router/web format   # oxfmt src/
```

### `router/api/` (Go module `luban`, binary `rui`)

```bash
cd router/api
go build ./...
go test ./...
```

## Hard Conventions

### Frontend styling: tv/twc only, no bare Tailwind in JSX

Every component's variants (size, color, state) are defined once via
`tailwind-variants` (`tv()`), then wrapped into a typed React component via
`react-twc` (`twc`). Pages compose these typed components; they never write
raw Tailwind utility classes directly in JSX. See `router/web/src/components/ui/`
for the existing pattern (`Button.tsx`, `Card.tsx`, `Alert.tsx`, etc.) before
adding new UI.

### Frontend data/state stack: zod + TanStack Query + TanStack Form + zustand

`router/web/src` is built on four libraries, each with one job — do not
reach for local `useState`/`useEffect` fetch-on-mount patterns or Context
providers where one of these already fits:

- **zod** (`src/api/schemas.ts`) — schema for every `/api/*` request and
  response. `src/api/types.ts` is a re-export shim (`z.infer` only); the Go
  struct in `router/api/internal/config/config.go` remains the ultimate
  source of truth (see "config.json is the single source of truth" above),
  but the zod schema is what catches drift at runtime — a shape mismatch
  fails loudly with a console error naming the field, not a silent
  `undefined` in the UI (the exact failure mode logged in the Field-Test Log
  under "Frontend types written against an unimplemented API shape").
- **@tanstack/react-query** (`src/api/queries.ts`, `src/api/mutations.ts`) —
  all reads are `useQuery` (config/status/health/log), all writes are
  `useMutation` (saveConfig/password/apply/confirm/rollback/restartService).
  Status and health keep polling via `refetchInterval`; nothing else does.
  **Optimistic cache updates (`onMutate`/`onError`/`onSettled`) apply ONLY to
  `saveConfig`** (the single PUT `/api/config` endpoint that covers both
  regular WAN/LAN/MTU form submits and DNS upstream-list edits — see
  `useSaveConfigMutation` in `src/api/mutations.ts`). `apply`/`confirm`/
  `rollback` are never optimistic: `ApplyDialog`'s countdown/poll/auto-confirm
  state machine needs its real, server-confirmed state at every step, not an
  assumed one. Every mutation that invalidates a query must `return` the
  `invalidateQueries(...)` promise from `onSettled`/`onSuccess` (not
  fire-and-forget it) so the mutation stays `pending` until the refetch
  actually completes — this is the official TanStack Query v5 guidance, not
  a house style choice.
- **@tanstack/react-form** — every form (login, password-change, WAN/LAN/MTU
  sections, DNS upstream list) uses `useForm` with a zod schema passed
  directly as the `validators.onChange`/`onSubmit` value (TanStack Form
  implements the Standard Schema spec — no adapter package needed). Field
  errors render through the shared `fieldErrorText()` helper
  (`src/lib/formSchemas.ts`), which normalizes both a zod issue's `.message`
  and a bare-string function-validator error (used by the MTU/MSS section,
  whose valid range depends on external WAN-mode state, not a static
  schema) into one string.
- **zustand** — `src/store/authStore.ts` (isAuthenticated/authChecked/401
  wiring) and `src/store/applyDialogStore.ts` (ApplyDialog's state machine).
  Replaces the old Context-based `AuthProvider`. Actions are referentially
  stable across renders — safe to put directly in a `useEffect`/`useCallback`
  dependency array.

`src/Providers.tsx` wraps the app in a single SSR-aware `QueryClient`
singleton (`isServer` branch kept for future-proofing even though this is a
client-only SPA today — swapping to a server-rendering setup later shouldn't
require rewriting this file). Default `staleTime` is 60s; per-query overrides
(e.g. status/health's `refetchInterval`) are unaffected.

### Frontend file-size and component limits

Enforced by `oxlint`'s `max-lines` rule in `router/web/.oxlintrc.json`
(error severity, 300-line cap, comments/blank lines excluded) — `pnpm lint`
fails the build if any file in `router/web/src` exceeds it. Split by
extracting page sections into their own components (see
`src/pages/network/{WanSection,LanSection,MtuSection}.tsx` +
`src/pages/network/TextField.tsx` for the shared field-binding boilerplate)
before reaching for `useState`-fest 800-line page files.

One default-exported/named React **component definition** per file — the
limit is on component definitions, not on constants/hooks/types/utility
functions living alongside one (a file can freely export a component plus
its props type, a default-config constant, etc.). **Exemption**: a tightly
coupled "compound component" kit — several one-tag `twc` styling primitives
with no independent logic, in the shadcn/radix compound-component tradition
(e.g. `components/ui/Card.tsx`'s Card/CardHeader/CardBody/CardFooter/
CardTitle, or a page's local `primitives.tsx`) — may live in one file. A
barrel `index.ts` that only re-exports is always fine; the rule is about
definition files.

### react-twc: transient (`$`-prefixed) props are the only variant-prop convention

Every `tv()`-driven variant prop on a `twc`-wrapped component uses the `$`
prefix (e.g. `$type`, `$variant`, `$size`) and nothing else. This isn't a
manual `.transientProps()` call per component — react-twc's *default*
`shouldForwardProp` is `prop => prop[0] !== "$"`, so any `$`-prefixed prop is
automatically stripped before reaching the DOM node with zero extra code.
Do not add a differently-prefixed variant prop, and do not call
`.transientProps()` manually unless a specific component genuinely needs a
custom forwarding rule beyond the `$` convention.

### Templates are on-disk files, never `go:embed`

`router/templates/*.tpl` are Go `text/template` files read from disk at
runtime by `router/api/internal/apply`. Do not switch these to `go:embed` —
externalizing templates is what keeps the Go binary stable while service
config logic evolves without a recompile/redeploy cycle.

### `config.json` is the single source of truth — schema is a cross-cutting contract

The JSON field names in `config.json` are read by three independent
consumers: the Go structs in `router/api/internal/config/config.go`
(canonical definition), the frontend types in `router/web/src/api/types.ts`,
and the Go templates in `router/templates/*.tpl`. Any field rename, addition,
or type change must be applied to all three in the same change. The Go
structs are the source of truth if they ever disagree.

### Go nil slices marshal to JSON `null` — frontend must normalize

An empty/nil Go slice field serializes as `null`, not `[]`. Any frontend code
consuming array fields from `/api/*` responses must treat `null` as `[]`
(don't assume the API always returns an array for array-typed fields).

### Every generated config file gets a `.bak` sibling — this is the rollback contract

Whenever `router/api/internal/apply` writes a rendered service config (e.g.
`/etc/nftables.conf`, `/etc/dnsmasq.d/router.conf`), it must first back up
whatever file it's replacing to `<path>.bak` before overwriting. This is not
optional bookkeeping — `router/systemd/router-rollback.sh` restores directly
from these `.bak` siblings on rollback and has no other source of the
pre-apply state. Both sides (Go writer, shell restorer) must stay in sync;
see `router/api/internal/apply/rollback_contract_test.go` for the pattern
that encodes this as a test rather than a convention people have to remember.

### Never bypass apply → confirm → 90s rollback

Any code path that writes a rendered service config to its live location
(`/etc/...`) must go through the apply pipeline: dry-run validate → backup →
write + reload → start the 90s confirm timer. Do not add a shortcut that
writes a live config file directly, even for a single-field change — it
breaks the anti-lockout guarantee the whole design exists for.

### networkd `.network` files are first-match-wins by filename sort

systemd-networkd applies only the first matching `.network` file by filename
sort across `/usr/lib/systemd/network`, `/run/systemd/network`, and
`/etc/systemd/network` combined — not a merge. When adding or debugging an
interface, run `networkctl status <iface>` and check the "Network File" line
to confirm which file actually governs it before assuming luban's own
`.network` is in effect (see the cloud-init/netplan Field-Test Log entry in
DECISIONS.md for the failure mode this causes).

### Cross-component contracts must be tests, not comments

When a change spans Go structs, templates, and generated files (or the
apply/rollback shell script), add or extend a test that exercises the real
files rather than relying on a comment to keep them in sync. Existing
examples to follow:

- `router/api/internal/apply/render_real_test.go` — renders the actual
  on-disk templates against real config data and checks output, catching
  template/schema drift that a unit test with fixture strings would miss.
- `router/api/internal/apply/rollback_contract_test.go` — verifies the
  backup-file contract between the Go apply pipeline and
  `router/systemd/router-rollback.sh` stays intact.
