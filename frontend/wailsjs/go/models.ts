export namespace backend {
	
	export class CustomerPreviewItem {
	    Description: string;
	    DisplayDesc: string;
	    UoM: string;
	    DisplayUnitPrice: string;
	    DisplayTotal: string;
	    UnitPrice: number;
	    Total: number;
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
	        this.DisplayUnitPrice = source["DisplayUnitPrice"];
	        this.DisplayTotal = source["DisplayTotal"];
	        this.UnitPrice = source["UnitPrice"];
	        this.Total = source["Total"];
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
	    Date: string;
	    ValidUntil: string;
	    Incoterm: string;
	    DisplaySubTotal: string;
	    DisplayDiscountAmount: string;
	    DisplayGrandTotal: string;
	    SubTotal: number;
	    DiscountAmount: number;
	    GrandTotal: number;
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
	        this.Date = source["Date"];
	        this.ValidUntil = source["ValidUntil"];
	        this.Incoterm = source["Incoterm"];
	        this.DisplaySubTotal = source["DisplaySubTotal"];
	        this.DisplayDiscountAmount = source["DisplayDiscountAmount"];
	        this.DisplayGrandTotal = source["DisplayGrandTotal"];
	        this.SubTotal = source["SubTotal"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.GrandTotal = source["GrandTotal"];
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
	
	export class InternalPreviewItem {
	    Description: string;
	    CostUnit: number;
	    SellUnit: number;
	    CostTotal: number;
	    SellTotal: number;
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
	        this.No = source["No"];
	        this.Qty = source["Qty"];
	    }
	}
	export class InternalBreakdown {
	    Items: InternalPreviewItem[];
	    TaxLabel: string;
	    DisplayCost: string;
	    DisplaySell: string;
	    DisplayCostDisc: string;
	    DisplaySellDisc: string;
	    DisplayCostGrd: string;
	    DisplayGrand: string;
	    DisplayProfit: string;
	    DisplayTax: string;
	    DisplayNett: string;
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
	    MarginPercent: number;
	    DiscountPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new InternalBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Items = this.convertValues(source["Items"], InternalPreviewItem);
	        this.TaxLabel = source["TaxLabel"];
	        this.DisplayCost = source["DisplayCost"];
	        this.DisplaySell = source["DisplaySell"];
	        this.DisplayCostDisc = source["DisplayCostDisc"];
	        this.DisplaySellDisc = source["DisplaySellDisc"];
	        this.DisplayCostGrd = source["DisplayCostGrd"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.DisplayProfit = source["DisplayProfit"];
	        this.DisplayTax = source["DisplayTax"];
	        this.DisplayNett = source["DisplayNett"];
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
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
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
	
	export class QuotationItemInput {
	    Description: string;
	    Qty: number;
	    UoM: string;
	    UnitPrice: number;
	
	    static createFrom(source: any = {}) {
	        return new QuotationItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.Qty = source["Qty"];
	        this.UoM = source["UoM"];
	        this.UnitPrice = source["UnitPrice"];
	    }
	}
	export class QuotationInput {
	    Items: QuotationItemInput[];
	    Subject: string;
	    RecipientName: string;
	    DiscountPercent: number;
	    ProfitMargin: number;
	    TaxZone: string;
	    TaxCategory: string;
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
	        this.ProfitMargin = source["ProfitMargin"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
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

}

