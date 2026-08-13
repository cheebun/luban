// Zod schemas for /api/wizard/* endpoints. Kept separate from schemas.ts to
// avoid pushing that file over 300 LOC.
import { z } from "zod";

const nullableArray = <T extends z.ZodTypeAny>(item: T) =>
  z.array(item).nullish().transform((v) => v ?? []);

export const WizardBoardSchema = z.object({
  id: z.string(),
  name: z.string(),
});

export const WizardInterfaceSchema = z.object({
  name: z.string(),
  path: z.string(),
  mac: z.string(),
  link: z.boolean(),
  wifi: z.boolean(),
  role_suggestion: z.enum(["wan", "lan"]).nullable(),
  dhcp_offer: z.boolean().nullable(),
  offer_server: z.string().nullable(),
});

export const WizardStateSchema = z.object({
  configured: z.boolean(),
  board: WizardBoardSchema.nullable(),
  interfaces: nullableArray(WizardInterfaceSchema),
  probed: z.boolean(),
});

export const WizardProbeResponseSchema = z.object({
  interfaces: nullableArray(WizardInterfaceSchema),
});

export const WizardCompleteRequestSchema = z.object({
  wan_interface: z.string(),
  lan_interfaces: z.array(z.string()),
  lan_address: z.string().optional(),
  password: z.string(),
});

export const WizardCompleteResponseSchema = z.object({
  ok: z.boolean(),
  new_url: z.string().optional(),
});

export type WizardBoard = z.infer<typeof WizardBoardSchema>;
export type WizardInterface = z.infer<typeof WizardInterfaceSchema>;
export type WizardState = z.infer<typeof WizardStateSchema>;
export type WizardProbeResponse = z.infer<typeof WizardProbeResponseSchema>;
export type WizardCompleteRequest = z.infer<typeof WizardCompleteRequestSchema>;
export type WizardCompleteResponse = z.infer<typeof WizardCompleteResponseSchema>;
