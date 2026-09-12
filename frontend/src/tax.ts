export interface TaxZoneOption {
  value: "FTZ" | "NonFTZ";
  label: string;
}

export interface TaxCategoryOption {
  value: "SupplyOnly" | "SupplyInstall" | "SupplyInstallConstruction";
  label: string;
  rateBps: number;
}

export const TAX_ZONE_OPTIONS: readonly TaxZoneOption[] = [
  { value: "FTZ", label: "FTZ" },
  { value: "NonFTZ", label: "Non-FTZ" },
] as const;

export const TAX_CATEGORY_OPTIONS: readonly TaxCategoryOption[] = [
  { value: "SupplyOnly", label: "Supply only", rateBps: 1100 },
  { value: "SupplyInstall", label: "Supply and install services", rateBps: 200 },
  {
    value: "SupplyInstallConstruction",
    label: "Supply and install services — construction",
    rateBps: 175,
  },
] as const;

/**
 * Normalizes a pasted tax zone. Accepts the stored value, the display label,
 * or a rate; anything that is not clearly Non-FTZ is treated as FTZ.
 */
export function normalizeTaxZone(raw: string): "FTZ" | "NonFTZ" {
  const value = raw.toLowerCase().replace(/[^a-z]/g, "");
  if (value.includes("non")) return "NonFTZ";
  return "FTZ";
}

/**
 * Normalizes a pasted tax category. Accepts stored values, human labels, and
 * bare rates such as "11%", "2%", or "1.75%". Returns "" when unrecognized.
 */
export function normalizeTaxCategory(raw: string): string {
  const value = raw.toLowerCase().replace(/[^a-z0-9.]/g, "");
  if (!value) return "";
  if (value.includes("construction")) return "SupplyInstallConstruction";
  if (value.includes("install")) return "SupplyInstall";
  if (value.includes("supply")) return "SupplyOnly";
  if (value.includes("1.75") || value.includes("175")) return "SupplyInstallConstruction";
  if (value.includes("11")) return "SupplyOnly";
  if (value.includes("2")) return "SupplyInstall";
  return "";
}

export function taxRateBps(zone: string, category: string): number {
  if (zone !== "NonFTZ") return 0;
  const match = TAX_CATEGORY_OPTIONS.find((option) => option.value === category);
  return match?.rateBps ?? 0;
}

export function taxRateLabel(zone: string, category: string): string {
  const bps = taxRateBps(zone, category);
  if (bps === 0) return "0%";
  return `${(bps / 100).toLocaleString("id-ID", { maximumFractionDigits: 2 })}%`;
}
