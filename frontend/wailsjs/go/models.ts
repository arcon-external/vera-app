export namespace auth {
	
	export class Service {
	
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace backend {
	
	export class ArchivedQuotationItem {
	    ItemNo: number;
	    Description: string;
	    Qty: number;
	    UoM: string;
	    UnitPrice: number;
	    DisplayUnitPrice: string;
	    MarginEnabled: boolean;
	    MarginPercent: number;
	    MarginAmount: number;
	    DisplayMarginAmount: string;
	    OriginalTotal: number;
	    DisplayOriginalTotal: string;
	    MarginalizedUnitPrice: number;
	    DisplayMarginalizedUnit: string;
	    MarginalizedTotal: number;
	    DisplayMarginalizedTotal: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotationItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ItemNo = source["ItemNo"];
	        this.Description = source["Description"];
	        this.Qty = source["Qty"];
	        this.UoM = source["UoM"];
	        this.UnitPrice = source["UnitPrice"];
	        this.DisplayUnitPrice = source["DisplayUnitPrice"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.MarginPercent = source["MarginPercent"];
	        this.MarginAmount = source["MarginAmount"];
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	    }
	}
	export class ArchivedQuotation {
	    ID: number;
	    QuoteNumber: string;
	    RevisionNumber: number;
	    CustomerName: string;
	    Subject: string;
	    CreatedAt: string;
	    GrandTotal: number;
	    DisplayGrand: string;
	    CustomerCode: number;
	    CustomerAddress: string;
	    RecipientName: string;
	    RecipientAddress: string;
	    Incoterm: string;
	    ValidityDays: number;
	    PaymentTermDays: number;
	    LeadTimeDays: number;
	    DiscountPercent: number;
	    TaxZone: string;
	    TaxCategory: string;
	    TaxType: string;
	    ServiceCategory: string;
	    TaxRateBps: number;
	    OriginalSubtotal: number;
	    OriginalDiscount: number;
	    OriginalGrand: number;
	    MarginalizedSubtotal: number;
	    MarginalizedDiscount: number;
	    MarginalizedGrand: number;
	    PpnAmount: number;
	    CustomerGrandTotal: number;
	    DisplayPpn: string;
	    TaxAmount: number;
	    MarginalizedAfterTax: number;
	    ProfitAmount: number;
	    NettProfit: number;
	    DisplayOriginalSubtotal: string;
	    DisplayOriginalDiscount: string;
	    DisplayOriginalGrand: string;
	    DisplayMarginalizedSubtotal: string;
	    DisplayMarginalizedDiscount: string;
	    DisplayMarginalizedGrand: string;
	    DisplayTax: string;
	    DisplayMarginalizedAfterTax: string;
	    DisplayProfit: string;
	    DisplayNett: string;
	    Items: ArchivedQuotationItem[];
	    HTMLSnapshot: string;
	    AvailableRevisions: number[];
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.Subject = source["Subject"];
	        this.CreatedAt = source["CreatedAt"];
	        this.GrandTotal = source["GrandTotal"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.CustomerCode = source["CustomerCode"];
	        this.CustomerAddress = source["CustomerAddress"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.Incoterm = source["Incoterm"];
	        this.ValidityDays = source["ValidityDays"];
	        this.PaymentTermDays = source["PaymentTermDays"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.TaxRateBps = source["TaxRateBps"];
	        this.OriginalSubtotal = source["OriginalSubtotal"];
	        this.OriginalDiscount = source["OriginalDiscount"];
	        this.OriginalGrand = source["OriginalGrand"];
	        this.MarginalizedSubtotal = source["MarginalizedSubtotal"];
	        this.MarginalizedDiscount = source["MarginalizedDiscount"];
	        this.MarginalizedGrand = source["MarginalizedGrand"];
	        this.PpnAmount = source["PpnAmount"];
	        this.CustomerGrandTotal = source["CustomerGrandTotal"];
	        this.DisplayPpn = source["DisplayPpn"];
	        this.TaxAmount = source["TaxAmount"];
	        this.MarginalizedAfterTax = source["MarginalizedAfterTax"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.NettProfit = source["NettProfit"];
	        this.DisplayOriginalSubtotal = source["DisplayOriginalSubtotal"];
	        this.DisplayOriginalDiscount = source["DisplayOriginalDiscount"];
	        this.DisplayOriginalGrand = source["DisplayOriginalGrand"];
	        this.DisplayMarginalizedSubtotal = source["DisplayMarginalizedSubtotal"];
	        this.DisplayMarginalizedDiscount = source["DisplayMarginalizedDiscount"];
	        this.DisplayMarginalizedGrand = source["DisplayMarginalizedGrand"];
	        this.DisplayTax = source["DisplayTax"];
	        this.DisplayMarginalizedAfterTax = source["DisplayMarginalizedAfterTax"];
	        this.DisplayProfit = source["DisplayProfit"];
	        this.DisplayNett = source["DisplayNett"];
	        this.Items = this.convertValues(source["Items"], ArchivedQuotationItem);
	        this.HTMLSnapshot = source["HTMLSnapshot"];
	        this.AvailableRevisions = source["AvailableRevisions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ArchivedQuotationSummary {
	    ID: number;
	    QuoteNumber: string;
	    RevisionNumber: number;
	    CustomerName: string;
	    Subject: string;
	    CreatedAt: string;
	    GrandTotal: number;
	    DisplayGrand: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotationSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.Subject = source["Subject"];
	        this.CreatedAt = source["CreatedAt"];
	        this.GrandTotal = source["GrandTotal"];
	        this.DisplayGrand = source["DisplayGrand"];
	    }
	}
	export class AuthUserDTO {
	    ID: number;
	    Email: string;
	    DisplayName: string;
	    Role: string;
	    IsActive: boolean;
	    MustChangePassword: boolean;
	    LastLoginAt: string;
	    AvatarURL: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthUserDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Role = source["Role"];
	        this.IsActive = source["IsActive"];
	        this.MustChangePassword = source["MustChangePassword"];
	        this.LastLoginAt = source["LastLoginAt"];
	        this.AvatarURL = source["AvatarURL"];
	    }
	}
	export class CustomerPreviewItem {
	    Description: string;
	    DisplayDesc: string;
	    UoM: string;
	    DisplayOriginalUnitPrice: string;
	    DisplayOriginalTotal: string;
	    DisplayUnitPrice: string;
	    DisplayTotal: string;
	    OriginalUnitPrice: number;
	    OriginalTotal: number;
	    UnitPrice: number;
	    Total: number;
	    MarginEnabled: boolean;
	    MarginPercent: number;
	    MarginAmount: number;
	    No: number;
	    Qty: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomerPreviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.DisplayDesc = source["DisplayDesc"];
	        this.UoM = source["UoM"];
	        this.DisplayOriginalUnitPrice = source["DisplayOriginalUnitPrice"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.DisplayUnitPrice = source["DisplayUnitPrice"];
	        this.DisplayTotal = source["DisplayTotal"];
	        this.OriginalUnitPrice = source["OriginalUnitPrice"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.UnitPrice = source["UnitPrice"];
	        this.Total = source["Total"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.MarginPercent = source["MarginPercent"];
	        this.MarginAmount = source["MarginAmount"];
	        this.No = source["No"];
	        this.Qty = source["Qty"];
	    }
	}
	export class CustomerPreview {
	    Items: CustomerPreviewItem[];
	    Subject: string;
	    RecipientName: string;
	    RecipientCompany: string;
	    RecipientAddress: string;
	    QuoteNumber: string;
	    RevisionID: number;
	    Date: string;
	    ValidUntil: string;
	    Incoterm: string;
	    DisplaySubTotal: string;
	    DisplayDiscountAmount: string;
	    DisplayGrandTotal: string;
	    DisplayPpnAmount: string;
	    SubTotal: number;
	    DiscountAmount: number;
	    GrandTotal: number;
	    PpnAmount: number;
	    OriginalSubTotal: number;
	    OriginalDiscountAmount: number;
	    OriginalGrandTotal: number;
	    MarginalizedGrandAfterTax: number;
	    ProfitAmount: number;
	    NettProfit: number;
	    DiscountPercent: number;
	    Validity: number;
	    PaymentTerm: number;
	    LeadTime: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomerPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Items = this.convertValues(source["Items"], CustomerPreviewItem);
	        this.Subject = source["Subject"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientCompany = source["RecipientCompany"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.RevisionID = source["RevisionID"];
	        this.Date = source["Date"];
	        this.ValidUntil = source["ValidUntil"];
	        this.Incoterm = source["Incoterm"];
	        this.DisplaySubTotal = source["DisplaySubTotal"];
	        this.DisplayDiscountAmount = source["DisplayDiscountAmount"];
	        this.DisplayGrandTotal = source["DisplayGrandTotal"];
	        this.DisplayPpnAmount = source["DisplayPpnAmount"];
	        this.SubTotal = source["SubTotal"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.GrandTotal = source["GrandTotal"];
	        this.PpnAmount = source["PpnAmount"];
	        this.OriginalSubTotal = source["OriginalSubTotal"];
	        this.OriginalDiscountAmount = source["OriginalDiscountAmount"];
	        this.OriginalGrandTotal = source["OriginalGrandTotal"];
	        this.MarginalizedGrandAfterTax = source["MarginalizedGrandAfterTax"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.NettProfit = source["NettProfit"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.Validity = source["Validity"];
	        this.PaymentTerm = source["PaymentTerm"];
	        this.LeadTime = source["LeadTime"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DeletionRequest {
	    ID: number;
	    UserID: number;
	    Email: string;
	    DisplayName: string;
	    Status: string;
	    RequestedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DeletionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.UserID = source["UserID"];
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Status = source["Status"];
	        this.RequestedAt = source["RequestedAt"];
	    }
	}
	export class InternalPreviewItem {
	    Description: string;
	    CostUnit: number;
	    SellUnit: number;
	    CostTotal: number;
	    SellTotal: number;
	    OriginalUnitPrice: number;
	    OriginalTotal: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	    DisplayOriginalUnitPrice: string;
	    DisplayOriginalTotal: string;
	    DisplayMarginalizedUnit: string;
	    DisplayMarginalizedTotal: string;
	    DisplayMarginAmount: string;
	    MarginEnabled: boolean;
	    MarginPercent: number;
	    MarginAmount: number;
	    No: number;
	    Qty: number;
	
	    static createFrom(source: any = {}) {
	        return new InternalPreviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.CostUnit = source["CostUnit"];
	        this.SellUnit = source["SellUnit"];
	        this.CostTotal = source["CostTotal"];
	        this.SellTotal = source["SellTotal"];
	        this.OriginalUnitPrice = source["OriginalUnitPrice"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.DisplayOriginalUnitPrice = source["DisplayOriginalUnitPrice"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.MarginPercent = source["MarginPercent"];
	        this.MarginAmount = source["MarginAmount"];
	        this.No = source["No"];
	        this.Qty = source["Qty"];
	    }
	}
	export class InternalBreakdown {
	    Items: InternalPreviewItem[];
	    TaxLabel: string;
	    TaxType: string;
	    ServiceCategory: string;
	    PpnAmount: number;
	    DisplayPpn: string;
	    DisplayCost: string;
	    DisplaySell: string;
	    DisplayCostDisc: string;
	    DisplaySellDisc: string;
	    DisplayCostGrd: string;
	    DisplayGrand: string;
	    DisplayProfit: string;
	    DisplayTax: string;
	    DisplayNett: string;
	    DisplayOriginalSubtotal: string;
	    DisplayOriginalDiscount: string;
	    DisplayOriginalGrand: string;
	    DisplayMarginalizedSubtotal: string;
	    DisplayMarginalizedDiscount: string;
	    DisplayMarginalizedGrand: string;
	    DisplayMarginalizedAfterTax: string;
	    CostSubtotal: number;
	    SellSubtotal: number;
	    DiscountCost: number;
	    DiscountSell: number;
	    CostGrand: number;
	    GrandTotal: number;
	    ProfitAmount: number;
	    TaxAmount: number;
	    NettProfit: number;
	    TaxRateBps: number;
	    DiscountPercent: number;
	    OriginalSubtotal: number;
	    OriginalDiscount: number;
	    OriginalGrand: number;
	    MarginalizedSubtotal: number;
	    MarginalizedDiscount: number;
	    MarginalizedGrand: number;
	    MarginalizedGrandAfterTax: number;
	
	    static createFrom(source: any = {}) {
	        return new InternalBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Items = this.convertValues(source["Items"], InternalPreviewItem);
	        this.TaxLabel = source["TaxLabel"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.PpnAmount = source["PpnAmount"];
	        this.DisplayPpn = source["DisplayPpn"];
	        this.DisplayCost = source["DisplayCost"];
	        this.DisplaySell = source["DisplaySell"];
	        this.DisplayCostDisc = source["DisplayCostDisc"];
	        this.DisplaySellDisc = source["DisplaySellDisc"];
	        this.DisplayCostGrd = source["DisplayCostGrd"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.DisplayProfit = source["DisplayProfit"];
	        this.DisplayTax = source["DisplayTax"];
	        this.DisplayNett = source["DisplayNett"];
	        this.DisplayOriginalSubtotal = source["DisplayOriginalSubtotal"];
	        this.DisplayOriginalDiscount = source["DisplayOriginalDiscount"];
	        this.DisplayOriginalGrand = source["DisplayOriginalGrand"];
	        this.DisplayMarginalizedSubtotal = source["DisplayMarginalizedSubtotal"];
	        this.DisplayMarginalizedDiscount = source["DisplayMarginalizedDiscount"];
	        this.DisplayMarginalizedGrand = source["DisplayMarginalizedGrand"];
	        this.DisplayMarginalizedAfterTax = source["DisplayMarginalizedAfterTax"];
	        this.CostSubtotal = source["CostSubtotal"];
	        this.SellSubtotal = source["SellSubtotal"];
	        this.DiscountCost = source["DiscountCost"];
	        this.DiscountSell = source["DiscountSell"];
	        this.CostGrand = source["CostGrand"];
	        this.GrandTotal = source["GrandTotal"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.TaxAmount = source["TaxAmount"];
	        this.NettProfit = source["NettProfit"];
	        this.TaxRateBps = source["TaxRateBps"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.OriginalSubtotal = source["OriginalSubtotal"];
	        this.OriginalDiscount = source["OriginalDiscount"];
	        this.OriginalGrand = source["OriginalGrand"];
	        this.MarginalizedSubtotal = source["MarginalizedSubtotal"];
	        this.MarginalizedDiscount = source["MarginalizedDiscount"];
	        this.MarginalizedGrand = source["MarginalizedGrand"];
	        this.MarginalizedGrandAfterTax = source["MarginalizedGrandAfterTax"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ManagedUser {
	    ID: number;
	    Email: string;
	    DisplayName: string;
	    Role: string;
	    IsActive: boolean;
	    MustChangePassword: boolean;
	    LastLoginAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ManagedUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Role = source["Role"];
	        this.IsActive = source["IsActive"];
	        this.MustChangePassword = source["MustChangePassword"];
	        this.LastLoginAt = source["LastLoginAt"];
	    }
	}
	export class QuotationItemInput {
	    Description: string;
	    Qty: number;
	    UoM: string;
	    UnitPrice: number;
	    MarginEnabled: boolean;
	    MarginPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new QuotationItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.Qty = source["Qty"];
	        this.UoM = source["UoM"];
	        this.UnitPrice = source["UnitPrice"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.MarginPercent = source["MarginPercent"];
	    }
	}
	export class QuotationInput {
	    Items: QuotationItemInput[];
	    Subject: string;
	    RecipientName: string;
	    DiscountPercent: number;
	    TaxZone: string;
	    TaxCategory: string;
	    TaxType: string;
	    ServiceCategory: string;
	    Validity: number;
	    PaymentTerm: number;
	    LeadTime: number;
	    CustomerID: number;
	    Incoterm: string;
	
	    static createFrom(source: any = {}) {
	        return new QuotationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Items = this.convertValues(source["Items"], QuotationItemInput);
	        this.Subject = source["Subject"];
	        this.RecipientName = source["RecipientName"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.Validity = source["Validity"];
	        this.PaymentTerm = source["PaymentTerm"];
	        this.LeadTime = source["LeadTime"];
	        this.CustomerID = source["CustomerID"];
	        this.Incoterm = source["Incoterm"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RestoreResult {
	    SafetySnapshot: string;
	    Quotations: number;
	    Revisions: number;
	
	    static createFrom(source: any = {}) {
	        return new RestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SafetySnapshot = source["SafetySnapshot"];
	        this.Quotations = source["Quotations"];
	        this.Revisions = source["Revisions"];
	    }
	}

}

export namespace store {
	
	export class RevisionTotals {
	    OriginalSubtotal: number;
	    OriginalDiscount: number;
	    OriginalGrand: number;
	    MarginalizedSubtotal: number;
	    MarginalizedDiscount: number;
	    MarginalizedGrand: number;
	    TaxAmount: number;
	    MarginalizedGrandAfterTax: number;
	    ProfitAmount: number;
	    NettProfit: number;
	    PpnAmount: number;
	    CustomerGrandTotal: number;
	
	    static createFrom(source: any = {}) {
	        return new RevisionTotals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.OriginalSubtotal = source["OriginalSubtotal"];
	        this.OriginalDiscount = source["OriginalDiscount"];
	        this.OriginalGrand = source["OriginalGrand"];
	        this.MarginalizedSubtotal = source["MarginalizedSubtotal"];
	        this.MarginalizedDiscount = source["MarginalizedDiscount"];
	        this.MarginalizedGrand = source["MarginalizedGrand"];
	        this.TaxAmount = source["TaxAmount"];
	        this.MarginalizedGrandAfterTax = source["MarginalizedGrandAfterTax"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.NettProfit = source["NettProfit"];
	        this.PpnAmount = source["PpnAmount"];
	        this.CustomerGrandTotal = source["CustomerGrandTotal"];
	    }
	}
	export class RevisionItem {
	    ItemNo: number;
	    Description: string;
	    Qty: number;
	    UoM: string;
	    UnitPrice: number;
	    MarginEnabled: boolean;
	    MarginPercent: number;
	    MarginAmount: number;
	    OriginalTotal: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	
	    static createFrom(source: any = {}) {
	        return new RevisionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ItemNo = source["ItemNo"];
	        this.Description = source["Description"];
	        this.Qty = source["Qty"];
	        this.UoM = source["UoM"];
	        this.UnitPrice = source["UnitPrice"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.MarginPercent = source["MarginPercent"];
	        this.MarginAmount = source["MarginAmount"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	    }
	}
	export class RevisionDraft {
	    QuoteNumber: string;
	    CustomerCode: number;
	    CustomerName: string;
	    CustomerAddress: string;
	    RevisionNumber: number;
	    RecipientName: string;
	    RecipientAddress: string;
	    Subject: string;
	    Incoterm: string;
	    ValidityDays: number;
	    PaymentTermDays: number;
	    LeadTimeDays: number;
	    DiscountPercent: number;
	    TaxZone: string;
	    TaxCategory: string;
	    TaxType: string;
	    ServiceCategory: string;
	    TaxRateBps: number;
	    Items: RevisionItem[];
	    Totals: RevisionTotals;
	    HTMLSnapshot: string;
	    PDFData: number[];
	    PDFPath: string;
	    DocumentHash: string;
	
	    static createFrom(source: any = {}) {
	        return new RevisionDraft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerCode = source["CustomerCode"];
	        this.CustomerName = source["CustomerName"];
	        this.CustomerAddress = source["CustomerAddress"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.Subject = source["Subject"];
	        this.Incoterm = source["Incoterm"];
	        this.ValidityDays = source["ValidityDays"];
	        this.PaymentTermDays = source["PaymentTermDays"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.TaxRateBps = source["TaxRateBps"];
	        this.Items = this.convertValues(source["Items"], RevisionItem);
	        this.Totals = this.convertValues(source["Totals"], RevisionTotals);
	        this.HTMLSnapshot = source["HTMLSnapshot"];
	        this.PDFData = source["PDFData"];
	        this.PDFPath = source["PDFPath"];
	        this.DocumentHash = source["DocumentHash"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Store {
	
	
	    static createFrom(source: any = {}) {
	        return new Store(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

