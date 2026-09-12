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
