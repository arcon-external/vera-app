export namespace backend {
	
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

