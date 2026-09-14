export interface TaxZoneOption {
  value: "FTZ" | "NonFTZ";
  label: string;
}

export interface TaxTypeOption {
  value: "Goods" | "Services";
  label: string;
}

export interface ServiceCategoryOption {
  value: "Construction" | "NonConstruction";
  label: string;
  rateBps: number;
}

export const TAX_ZONE_OPTIONS: readonly TaxZoneOption[] = [
  { value: "FTZ", label: "FTZ" },
  { value: "NonFTZ", label: "Non-FTZ" },
] as const;

export const TAX_TYPE_OPTIONS: readonly TaxTypeOption[] = [
  { value: "Goods", label: "Goods" },
  { value: "Services", label: "Services" },
] as const;

export const SERVICE_CATEGORY_OPTIONS: readonly ServiceCategoryOption[] = [
  { value: "NonConstruction", label: "Non-construction", rateBps: 200 },
  { value: "Construction", label: "Construction", rateBps: 175 },
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
 * Normalizes a pasted tax type. Accepts stored values and human labels;
 * anything mentioning services wins, otherwise goods. Returns "" when blank.
 */
export function normalizeTaxType(raw: string): string {
  const value = raw.toLowerCase().replace(/[^a-z]/g, "");
  if (!value) return "";
  if (value.includes("service") || value.includes("jasa")) return "Services";
  if (value.includes("good") || value.includes("barang") || value.includes("supply")) {
    return "Goods";
  }
  return "";
}

/**
 * Normalizes a pasted service category. Accepts stored values, human labels,
 * and bare rates such as "2%" or "1.75%". Returns "" when unrecognized.
 */
export function normalizeServiceCategory(raw: string): string {
  const value = raw.toLowerCase().replace(/[^a-z0-9.]/g, "");
  if (!value) return "";
  if (value.includes("non") && value.includes("constr")) return "NonConstruction";
  if (value.includes("constr")) return "Construction";
  if (value.includes("1.75") || value.includes("175")) return "Construction";
  if (value.includes("service")) return "NonConstruction";
  return "";
}

/** PPN rate in bps: 11% for non-FTZ goods, otherwise 0 (customer-facing). */
export function ppnRateBps(zone: string, taxType: string): number {
  return zone === "NonFTZ" && taxType === "Goods" ? 1100 : 0;
}

/** Internal services rate in bps: 2% / 1.75%, otherwise 0. Never customer-facing. */
export function servicesRateBps(taxType: string, serviceCategory: string): number {
  if (taxType !== "Services") return 0;
  const match = SERVICE_CATEGORY_OPTIONS.find((option) => option.value === serviceCategory);
  return match?.rateBps ?? 0;
}

/**
 * Maps pre-type-model categories forward for archive rows created before
 * the Goods/Services split. Unknown values fall back to Goods.
 */
export function legacyTaxType(category: string): string {
  if (category === "SupplyInstall" || category === "SupplyInstallConstruction") {
    return "Services";
  }
  return "Goods";
}

export function legacyServiceCategory(category: string): string {
  if (category === "SupplyInstall") return "NonConstruction";
  if (category === "SupplyInstallConstruction") return "Construction";
  return "";
}

function formatBps(bps: number): string {
  if (bps === 0) return "0%";
  return `${(bps / 100).toLocaleString("id-ID", { maximumFractionDigits: 2 })}%`;
}

export function taxRateLabel(zone: string, taxType: string, serviceCategory: string): string {
  const ppn = ppnRateBps(zone, taxType);
  if (ppn > 0) return `PPN ${formatBps(ppn)}`;
  const services = servicesRateBps(taxType, serviceCategory);
  if (services > 0) {
    const kind =
      serviceCategory === "Construction" ? "final" : "withholding";
    return `${formatBps(services)} ${kind}`;
  }
  return "0%";
}
