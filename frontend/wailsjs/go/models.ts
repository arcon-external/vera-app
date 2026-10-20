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
	
	export class ArchivedQuotationSubItem {
	    DisplayMarginAmount: string;
	    Description: string;
	    Details: string[];
	    DisplayMarginalizedTotal: string;
	    UoM: string;
	    DisplayMarginalizedUnit: string;
	    DisplayOriginalUnit: string;
	    DisplayOriginalTotal: string;
	    MarginAmount: number;
	    DiscountAmount: number;
	    ListUnitPrice: number;
	    SubNo: number;
	    OriginalUnitPrice: number;
	    OriginalTotal: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	    Qty: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotationSubItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.Description = source["Description"];
	        this.Details = source["Details"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	        this.UoM = source["UoM"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.DisplayOriginalUnit = source["DisplayOriginalUnit"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.MarginAmount = source["MarginAmount"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.ListUnitPrice = source["ListUnitPrice"];
	        this.SubNo = source["SubNo"];
	        this.OriginalUnitPrice = source["OriginalUnitPrice"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	    }
	}
	export class ArchivedQuotationItem {
	    DisplayMarginAmount: string;
	    Description: string;
	    DisplayMarginalizedTotal: string;
	    UoM: string;
	    DisplayMarginalizedUnit: string;
	    DisplayUnitPrice: string;
	    DisplayOriginalTotal: string;
	    Subs: ArchivedQuotationSubItem[];
	    MarginAmount: number;
	    DiscountAmount: number;
	    ListUnitPrice: number;
	    ItemNo: number;
	    OriginalTotal: number;
	    MarginalizedUnitPrice: number;
	    UnitPrice: number;
	    MarginalizedTotal: number;
	    Qty: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	    ShowSubPrices: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotationItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.Description = source["Description"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	        this.UoM = source["UoM"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.DisplayUnitPrice = source["DisplayUnitPrice"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.Subs = this.convertValues(source["Subs"], ArchivedQuotationSubItem);
	        this.MarginAmount = source["MarginAmount"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.ListUnitPrice = source["ListUnitPrice"];
	        this.ItemNo = source["ItemNo"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.UnitPrice = source["UnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.ShowSubPrices = source["ShowSubPrices"];
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
	export class ArchivedQuotation {
	    DisplayPpn: string;
	    DisplayOriginalGrand: string;
	    DisplayOriginalDiscount: string;
	    RecipientName: string;
	    RecipientAddress: string;
	    Incoterm: string;
	    HTMLSnapshot: string;
	    DisplayNett: string;
	    DisplayProfit: string;
	    DisplayMarginalizedAfterTax: string;
	    TaxZone: string;
	    TaxCategory: string;
	    TaxType: string;
	    ServiceCategory: string;
	    DisplayTax: string;
	    DisplayMarginalizedGrand: string;
	    DisplayMarginalizedDiscount: string;
	    DisplayMarginalizedSubtotal: string;
	    DisplayOriginalSubtotal: string;
	    CustomerAddress: string;
	    Items: ArchivedQuotationItem[];
	    AvailableRevisions: number[];
	    QuoteNumber: string;
	    CustomerName: string;
	    Subject: string;
	    Currency: string;
	    CreatedAt: string;
	    DisplayGrand: string;
	    ID: number;
	    GrandTotal: number;
	    RevisionNumber: number;
	    OriginalGrand: number;
	    OriginalSubtotal: number;
	    MarginalizedAfterTax: number;
	    PpnAmount: number;
	    NettProfit: number;
	    MarginalizedSubtotal: number;
	    MarginalizedDiscount: number;
	    ProfitAmount: number;
	    OriginalDiscount: number;
	    TaxAmount: number;
	    MarginalizedGrand: number;
	    CustomerGrandTotal: number;
	    CustomerCode: number;
	    DisplayLeadTime: string;
	    LeadTimeYears: number;
	    LeadTimeMonths: number;
	    LeadTimeWeeks: number;
	    LeadTimeDays: number;
	    PaymentTermDays: number;
	    DiscountPercent: number;
	    ValidityDays: number;
	    TaxRateBps: number;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayPpn = source["DisplayPpn"];
	        this.DisplayOriginalGrand = source["DisplayOriginalGrand"];
	        this.DisplayOriginalDiscount = source["DisplayOriginalDiscount"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.Incoterm = source["Incoterm"];
	        this.HTMLSnapshot = source["HTMLSnapshot"];
	        this.DisplayNett = source["DisplayNett"];
	        this.DisplayProfit = source["DisplayProfit"];
	        this.DisplayMarginalizedAfterTax = source["DisplayMarginalizedAfterTax"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.DisplayTax = source["DisplayTax"];
	        this.DisplayMarginalizedGrand = source["DisplayMarginalizedGrand"];
	        this.DisplayMarginalizedDiscount = source["DisplayMarginalizedDiscount"];
	        this.DisplayMarginalizedSubtotal = source["DisplayMarginalizedSubtotal"];
	        this.DisplayOriginalSubtotal = source["DisplayOriginalSubtotal"];
	        this.CustomerAddress = source["CustomerAddress"];
	        this.Items = this.convertValues(source["Items"], ArchivedQuotationItem);
	        this.AvailableRevisions = source["AvailableRevisions"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.Subject = source["Subject"];
	        this.Currency = source["Currency"];
	        this.CreatedAt = source["CreatedAt"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.ID = source["ID"];
	        this.GrandTotal = source["GrandTotal"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.OriginalGrand = source["OriginalGrand"];
	        this.OriginalSubtotal = source["OriginalSubtotal"];
	        this.MarginalizedAfterTax = source["MarginalizedAfterTax"];
	        this.PpnAmount = source["PpnAmount"];
	        this.NettProfit = source["NettProfit"];
	        this.MarginalizedSubtotal = source["MarginalizedSubtotal"];
	        this.MarginalizedDiscount = source["MarginalizedDiscount"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.OriginalDiscount = source["OriginalDiscount"];
	        this.TaxAmount = source["TaxAmount"];
	        this.MarginalizedGrand = source["MarginalizedGrand"];
	        this.CustomerGrandTotal = source["CustomerGrandTotal"];
	        this.CustomerCode = source["CustomerCode"];
	        this.DisplayLeadTime = source["DisplayLeadTime"];
	        this.LeadTimeYears = source["LeadTimeYears"];
	        this.LeadTimeMonths = source["LeadTimeMonths"];
	        this.LeadTimeWeeks = source["LeadTimeWeeks"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.PaymentTermDays = source["PaymentTermDays"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.ValidityDays = source["ValidityDays"];
	        this.TaxRateBps = source["TaxRateBps"];
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
	    QuoteNumber: string;
	    CustomerName: string;
	    Subject: string;
	    Currency: string;
	    CreatedAt: string;
	    DisplayGrand: string;
	    ID: number;
	    GrandTotal: number;
	    RevisionNumber: number;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedQuotationSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.Subject = source["Subject"];
	        this.Currency = source["Currency"];
	        this.CreatedAt = source["CreatedAt"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.ID = source["ID"];
	        this.GrandTotal = source["GrandTotal"];
	        this.RevisionNumber = source["RevisionNumber"];
	    }
	}
	export class AuthUserDTO {
	    Email: string;
	    DisplayName: string;
	    Role: string;
	    LastLoginAt: string;
	    AvatarURL: string;
	    ID: number;
	    IsActive: boolean;
	    MustChangePassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuthUserDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Role = source["Role"];
	        this.LastLoginAt = source["LastLoginAt"];
	        this.AvatarURL = source["AvatarURL"];
	        this.ID = source["ID"];
	        this.IsActive = source["IsActive"];
	        this.MustChangePassword = source["MustChangePassword"];
	    }
	}
	export class CompanyLabelEntry {
	    Key: string;
	    Value: string;
	    Default: string;
	    MaxLength: number;
	    Terms: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CompanyLabelEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Key = source["Key"];
	        this.Value = source["Value"];
	        this.Default = source["Default"];
	        this.MaxLength = source["MaxLength"];
	        this.Terms = source["Terms"];
	    }
	}
	export class CompanyLogo {
	    LogoBase64: string;
	    LogoMime: string;
	    BuiltIn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CompanyLogo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.LogoBase64 = source["LogoBase64"];
	        this.LogoMime = source["LogoMime"];
	        this.BuiltIn = source["BuiltIn"];
	    }
	}
	export class CompanyProfile {
	    ThemeQDeep: string;
	    ThemePSoft: string;
	    Phone: string;
	    Email: string;
	    TaxID: string;
	    ThemePInk: string;
	    ThemeQSoft: string;
	    LegalName: string;
	    Address: string;
	    ThemeQInk: string;
	    ThemePDeep: string;
	    ThemePAccent: string;
	    ThemeQAccent: string;
	    Currency: string;
	    CanonicalUoM: string;
	    HasLogo: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CompanyProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ThemeQDeep = source["ThemeQDeep"];
	        this.ThemePSoft = source["ThemePSoft"];
	        this.Phone = source["Phone"];
	        this.Email = source["Email"];
	        this.TaxID = source["TaxID"];
	        this.ThemePInk = source["ThemePInk"];
	        this.ThemeQSoft = source["ThemeQSoft"];
	        this.LegalName = source["LegalName"];
	        this.Address = source["Address"];
	        this.ThemeQInk = source["ThemeQInk"];
	        this.ThemePDeep = source["ThemePDeep"];
	        this.ThemePAccent = source["ThemePAccent"];
	        this.ThemeQAccent = source["ThemeQAccent"];
	        this.Currency = source["Currency"];
	        this.CanonicalUoM = source["CanonicalUoM"];
	        this.HasLogo = source["HasLogo"];
	    }
	}
	export class CompanyUnit {
	    Value: string;
	    Label: string;
	
	    static createFrom(source: any = {}) {
	        return new CompanyUnit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Value = source["Value"];
	        this.Label = source["Label"];
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
	    DiscountEnabled: boolean;
	    DiscountPercent: number;
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
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.No = source["No"];
	        this.Qty = source["Qty"];
	    }
	}
	export class CustomerPreview {
	    Incoterm: string;
	    Subject: string;
	    RecipientName: string;
	    RecipientCompany: string;
	    RecipientAddress: string;
	    QuoteNumber: string;
	    Date: string;
	    ValidUntil: string;
	    DisplayPpnAmount: string;
	    DisplaySubTotal: string;
	    DisplayDiscountAmount: string;
	    DisplayGrandTotal: string;
	    Items: CustomerPreviewItem[];
	    SubTotal: number;
	    OriginalGrandTotal: number;
	    DiscountAmount: number;
	    GrandTotal: number;
	    PpnAmount: number;
	    OriginalSubTotal: number;
	    OriginalDiscountAmount: number;
	    RevisionID: number;
	    DiscountPercent: number;
	    DisplayLeadTime: string;
	    LeadTimeYears: number;
	    LeadTimeMonths: number;
	    LeadTimeWeeks: number;
	    LeadTimeDays: number;
	    Validity: number;
	    PaymentTerm: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomerPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Incoterm = source["Incoterm"];
	        this.Subject = source["Subject"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientCompany = source["RecipientCompany"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.Date = source["Date"];
	        this.ValidUntil = source["ValidUntil"];
	        this.DisplayPpnAmount = source["DisplayPpnAmount"];
	        this.DisplaySubTotal = source["DisplaySubTotal"];
	        this.DisplayDiscountAmount = source["DisplayDiscountAmount"];
	        this.DisplayGrandTotal = source["DisplayGrandTotal"];
	        this.Items = this.convertValues(source["Items"], CustomerPreviewItem);
	        this.SubTotal = source["SubTotal"];
	        this.OriginalGrandTotal = source["OriginalGrandTotal"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.GrandTotal = source["GrandTotal"];
	        this.PpnAmount = source["PpnAmount"];
	        this.OriginalSubTotal = source["OriginalSubTotal"];
	        this.OriginalDiscountAmount = source["OriginalDiscountAmount"];
	        this.RevisionID = source["RevisionID"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.DisplayLeadTime = source["DisplayLeadTime"];
	        this.LeadTimeYears = source["LeadTimeYears"];
	        this.LeadTimeMonths = source["LeadTimeMonths"];
	        this.LeadTimeWeeks = source["LeadTimeWeeks"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.Validity = source["Validity"];
	        this.PaymentTerm = source["PaymentTerm"];
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
	    Email: string;
	    DisplayName: string;
	    Status: string;
	    RequestedAt: string;
	    ID: number;
	    UserID: number;
	
	    static createFrom(source: any = {}) {
	        return new DeletionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Status = source["Status"];
	        this.RequestedAt = source["RequestedAt"];
	        this.ID = source["ID"];
	        this.UserID = source["UserID"];
	    }
	}
	export class DraftDTO {
	    Payload: string;
	    Subject: string;
	    CustomerName: string;
	    UpdatedAt: string;
	    Label: string;
	    Currency: string;
	    ID: number;
	    ItemCount: number;
	    CustomerID: number;
	
	    static createFrom(source: any = {}) {
	        return new DraftDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Payload = source["Payload"];
	        this.Subject = source["Subject"];
	        this.CustomerName = source["CustomerName"];
	        this.UpdatedAt = source["UpdatedAt"];
	        this.Label = source["Label"];
	        this.Currency = source["Currency"];
	        this.ID = source["ID"];
	        this.ItemCount = source["ItemCount"];
	        this.CustomerID = source["CustomerID"];
	    }
	}
	export class DraftSummaryDTO {
	    DisplayName: string;
	    Label: string;
	    Subject: string;
	    CustomerName: string;
	    UpdatedAt: string;
	    ID: number;
	    ItemCount: number;
	
	    static createFrom(source: any = {}) {
	        return new DraftSummaryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayName = source["DisplayName"];
	        this.Label = source["Label"];
	        this.Subject = source["Subject"];
	        this.CustomerName = source["CustomerName"];
	        this.UpdatedAt = source["UpdatedAt"];
	        this.ID = source["ID"];
	        this.ItemCount = source["ItemCount"];
	    }
	}
	export class EmailChangeRequest {
	    ProposedEmail: string;
	    Status: string;
	    RequestedAt: string;
	    ID: number;
	
	    static createFrom(source: any = {}) {
	        return new EmailChangeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ProposedEmail = source["ProposedEmail"];
	        this.Status = source["Status"];
	        this.RequestedAt = source["RequestedAt"];
	        this.ID = source["ID"];
	    }
	}
	export class EmailChangeResult {
	    Email: string;
	    PreviousEmail: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailChangeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Email = source["Email"];
	        this.PreviousEmail = source["PreviousEmail"];
	    }
	}
	export class EmailChangeRow {
	    Email: string;
	    DisplayName: string;
	    ProposedEmail: string;
	    Status: string;
	    RequestedAt: string;
	    ID: number;
	    UserID: number;
	
	    static createFrom(source: any = {}) {
	        return new EmailChangeRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.ProposedEmail = source["ProposedEmail"];
	        this.Status = source["Status"];
	        this.RequestedAt = source["RequestedAt"];
	        this.ID = source["ID"];
	        this.UserID = source["UserID"];
	    }
	}
	export class InternalPreviewSubItem {
	    Description: string;
	    DisplayOriginalUnitPrice: string;
	    DisplayMarginAmount: string;
	    DisplayMarginalizedTotal: string;
	    DisplayMarginalizedUnit: string;
	    DisplayOriginalTotal: string;
	    SubNo: string;
	    UoM: string;
	    Details: string[];
	    Qty: number;
	    MarginPercent: number;
	    OriginalUnitPrice: number;
	    OriginalTotal: number;
	    MarginAmount: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	    DiscountAmount: number;
	    ListUnitPrice: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InternalPreviewSubItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.DisplayOriginalUnitPrice = source["DisplayOriginalUnitPrice"];
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.SubNo = source["SubNo"];
	        this.UoM = source["UoM"];
	        this.Details = source["Details"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.OriginalUnitPrice = source["OriginalUnitPrice"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginAmount = source["MarginAmount"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.ListUnitPrice = source["ListUnitPrice"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	    }
	}
	export class InternalPreviewItem {
	    DisplayOriginalUnitPrice: string;
	    DisplayMarginAmount: string;
	    DisplayMarginalizedTotal: string;
	    DisplayMarginBlended: string;
	    DisplayMarginalizedUnit: string;
	    Description: string;
	    DisplayOriginalTotal: string;
	    SubItems: InternalPreviewSubItem[];
	    SellTotal: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	    OriginalTotal: number;
	    OriginalUnitPrice: number;
	    CostTotal: number;
	    SellUnit: number;
	    CostUnit: number;
	    MarginAmount: number;
	    MarginBlendedPercent: number;
	    No: number;
	    Qty: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	    MarginBlendedApprox: boolean;
	    ListUnitPrice: number;
	
	    static createFrom(source: any = {}) {
	        return new InternalPreviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayOriginalUnitPrice = source["DisplayOriginalUnitPrice"];
	        this.DisplayMarginAmount = source["DisplayMarginAmount"];
	        this.DisplayMarginalizedTotal = source["DisplayMarginalizedTotal"];
	        this.DisplayMarginBlended = source["DisplayMarginBlended"];
	        this.DisplayMarginalizedUnit = source["DisplayMarginalizedUnit"];
	        this.Description = source["Description"];
	        this.DisplayOriginalTotal = source["DisplayOriginalTotal"];
	        this.SubItems = this.convertValues(source["SubItems"], InternalPreviewSubItem);
	        this.SellTotal = source["SellTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.OriginalUnitPrice = source["OriginalUnitPrice"];
	        this.CostTotal = source["CostTotal"];
	        this.SellUnit = source["SellUnit"];
	        this.CostUnit = source["CostUnit"];
	        this.MarginAmount = source["MarginAmount"];
	        this.MarginBlendedPercent = source["MarginBlendedPercent"];
	        this.No = source["No"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.MarginBlendedApprox = source["MarginBlendedApprox"];
	        this.ListUnitPrice = source["ListUnitPrice"];
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
	export class InternalBreakdown {
	    DisplayMarginalizedGrand: string;
	    DisplayNett: string;
	    TaxType: string;
	    ServiceCategory: string;
	    DisplayMarginalizedAfterTax: string;
	    DisplayPpn: string;
	    DisplayCost: string;
	    DisplaySell: string;
	    DisplayCostDisc: string;
	    DisplaySellDisc: string;
	    DisplayCostGrd: string;
	    DisplayGrand: string;
	    DisplayProfit: string;
	    DisplayTax: string;
	    DisplayMarginalizedDiscount: string;
	    DisplayRowDiscountTotal: string;
	    DisplayOriginalSubtotal: string;
	    DisplayOriginalDiscount: string;
	    DisplayOriginalGrand: string;
	    TaxLabel: string;
	    DisplayMarginalizedSubtotal: string;
	    Items: InternalPreviewItem[];
	    NettProfit: number;
	    TaxAmount: number;
	    SellSubtotal: number;
	    MarginalizedGrandAfterTax: number;
	    PpnAmount: number;
	    CostGrand: number;
	    GrandTotal: number;
	    ProfitAmount: number;
	    CostSubtotal: number;
	    DiscountSell: number;
	    DiscountCost: number;
	    MarginalizedGrand: number;
	    OriginalSubtotal: number;
	    OriginalDiscount: number;
	    OriginalGrand: number;
	    MarginalizedSubtotal: number;
	    MarginalizedDiscount: number;
	    RowDiscountTotal: number;
	    TaxRateBps: number;
	    DiscountPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new InternalBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayMarginalizedGrand = source["DisplayMarginalizedGrand"];
	        this.DisplayNett = source["DisplayNett"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.DisplayMarginalizedAfterTax = source["DisplayMarginalizedAfterTax"];
	        this.DisplayPpn = source["DisplayPpn"];
	        this.DisplayCost = source["DisplayCost"];
	        this.DisplaySell = source["DisplaySell"];
	        this.DisplayCostDisc = source["DisplayCostDisc"];
	        this.DisplaySellDisc = source["DisplaySellDisc"];
	        this.DisplayCostGrd = source["DisplayCostGrd"];
	        this.DisplayGrand = source["DisplayGrand"];
	        this.DisplayProfit = source["DisplayProfit"];
	        this.DisplayTax = source["DisplayTax"];
	        this.DisplayMarginalizedDiscount = source["DisplayMarginalizedDiscount"];
	        this.DisplayRowDiscountTotal = source["DisplayRowDiscountTotal"];
	        this.DisplayOriginalSubtotal = source["DisplayOriginalSubtotal"];
	        this.DisplayOriginalDiscount = source["DisplayOriginalDiscount"];
	        this.DisplayOriginalGrand = source["DisplayOriginalGrand"];
	        this.TaxLabel = source["TaxLabel"];
	        this.DisplayMarginalizedSubtotal = source["DisplayMarginalizedSubtotal"];
	        this.Items = this.convertValues(source["Items"], InternalPreviewItem);
	        this.NettProfit = source["NettProfit"];
	        this.TaxAmount = source["TaxAmount"];
	        this.SellSubtotal = source["SellSubtotal"];
	        this.MarginalizedGrandAfterTax = source["MarginalizedGrandAfterTax"];
	        this.PpnAmount = source["PpnAmount"];
	        this.CostGrand = source["CostGrand"];
	        this.GrandTotal = source["GrandTotal"];
	        this.ProfitAmount = source["ProfitAmount"];
	        this.CostSubtotal = source["CostSubtotal"];
	        this.DiscountSell = source["DiscountSell"];
	        this.DiscountCost = source["DiscountCost"];
	        this.MarginalizedGrand = source["MarginalizedGrand"];
	        this.OriginalSubtotal = source["OriginalSubtotal"];
	        this.OriginalDiscount = source["OriginalDiscount"];
	        this.OriginalGrand = source["OriginalGrand"];
	        this.MarginalizedSubtotal = source["MarginalizedSubtotal"];
	        this.MarginalizedDiscount = source["MarginalizedDiscount"];
	        this.RowDiscountTotal = source["RowDiscountTotal"];
	        this.TaxRateBps = source["TaxRateBps"];
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
	
	
	export class ManagedClient {
	    Name: string;
	    PrimaryAddress: string;
	    ID: number;
	    QuotationCount: number;
	    Code: number;
	
	    static createFrom(source: any = {}) {
	        return new ManagedClient(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.PrimaryAddress = source["PrimaryAddress"];
	        this.ID = source["ID"];
	        this.QuotationCount = source["QuotationCount"];
	        this.Code = source["Code"];
	    }
	}
	export class ManagedUser {
	    Email: string;
	    DisplayName: string;
	    Role: string;
	    LastLoginAt: string;
	    ID: number;
	    IsActive: boolean;
	    MustChangePassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ManagedUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Email = source["Email"];
	        this.DisplayName = source["DisplayName"];
	        this.Role = source["Role"];
	        this.LastLoginAt = source["LastLoginAt"];
	        this.ID = source["ID"];
	        this.IsActive = source["IsActive"];
	        this.MustChangePassword = source["MustChangePassword"];
	    }
	}
	export class OwnDeletionRequest {
	    Status: string;
	    ID: number;
	
	    static createFrom(source: any = {}) {
	        return new OwnDeletionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Status = source["Status"];
	        this.ID = source["ID"];
	    }
	}
	export class POSourceActivePODTO {
	    ID: number;
	    State: string;
	
	    static createFrom(source: any = {}) {
	        return new POSourceActivePODTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.State = source["State"];
	    }
	}
	export class POSourceQuotationDTO {
	    ActivePO?: POSourceActivePODTO;
	    QuoteNumber: string;
	    Subject: string;
	    CustomerName: string;
	    ID: number;
	    LatestRevision: number;
	    IssuedRevisionCount: number;
	
	    static createFrom(source: any = {}) {
	        return new POSourceQuotationDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ActivePO = this.convertValues(source["ActivePO"], POSourceActivePODTO);
	        this.QuoteNumber = source["QuoteNumber"];
	        this.Subject = source["Subject"];
	        this.CustomerName = source["CustomerName"];
	        this.ID = source["ID"];
	        this.LatestRevision = source["LatestRevision"];
	        this.IssuedRevisionCount = source["IssuedRevisionCount"];
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
	export class POSourcePageDTO {
	    Quotations: POSourceQuotationDTO[];
	    Page: number;
	    PageSize: number;
	    Total: number;
	    TotalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new POSourcePageDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Quotations = this.convertValues(source["Quotations"], POSourceQuotationDTO);
	        this.Page = source["Page"];
	        this.PageSize = source["PageSize"];
	        this.Total = source["Total"];
	        this.TotalPages = source["TotalPages"];
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
	export class POSourceQuery {
	    Search: string;
	    Page: number;
	    HideUsed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new POSourceQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Search = source["Search"];
	        this.Page = source["Page"];
	        this.HideUsed = source["HideUsed"];
	    }
	}
	
	export class POSourceRevisionDTO {
	    IssuedAt: string;
	    Revision: number;
	
	    static createFrom(source: any = {}) {
	        return new POSourceRevisionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.IssuedAt = source["IssuedAt"];
	        this.Revision = source["Revision"];
	    }
	}
	export class POSourceRevisionsDTO {
	    QuoteNumber: string;
	    Subject: string;
	    CustomerName: string;
	    Revisions: POSourceRevisionDTO[];
	    QuotationID: number;
	
	    static createFrom(source: any = {}) {
	        return new POSourceRevisionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuoteNumber = source["QuoteNumber"];
	        this.Subject = source["Subject"];
	        this.CustomerName = source["CustomerName"];
	        this.Revisions = this.convertValues(source["Revisions"], POSourceRevisionDTO);
	        this.QuotationID = source["QuotationID"];
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
	export class POSourceSelection {
	    QuotationID: number;
	    RevisionNumber: number;
	
	    static createFrom(source: any = {}) {
	        return new POSourceSelection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuotationID = source["QuotationID"];
	        this.RevisionNumber = source["RevisionNumber"];
	    }
	}
	export class POSourceValidationDTO {
	    Reason: string;
	    QuotationID: number;
	    RevisionNumber: number;
	    Valid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new POSourceValidationDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Reason = source["Reason"];
	        this.QuotationID = source["QuotationID"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.Valid = source["Valid"];
	    }
	}
	export class PurchaseOrderCreateInput {
	    ReceivedDate: string;
	    QuotationID: number;
	    RevisionNumber: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderCreateInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ReceivedDate = source["ReceivedDate"];
	        this.QuotationID = source["QuotationID"];
	        this.RevisionNumber = source["RevisionNumber"];
	    }
	}
	export class PurchaseOrderItemDTO {
	    QuotationItemNo?: number;
	    QuotationSubNo?: number;
	    LineNumber: number;
	    Description: string;
	    QuantityScaled: number;
	    UoM: string;
	    UnitPriceMinor: number;
	    DiscountEnabled: boolean;
	    DiscountPercent: number;
	    GrossMinor: number;
	    DiscountMinor: number;
	    NetMinor: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderItemDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuotationItemNo = source["QuotationItemNo"];
	        this.QuotationSubNo = source["QuotationSubNo"];
	        this.LineNumber = source["LineNumber"];
	        this.Description = source["Description"];
	        this.QuantityScaled = source["QuantityScaled"];
	        this.UoM = source["UoM"];
	        this.UnitPriceMinor = source["UnitPriceMinor"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.GrossMinor = source["GrossMinor"];
	        this.DiscountMinor = source["DiscountMinor"];
	        this.NetMinor = source["NetMinor"];
	    }
	}
	export class PurchaseOrderDTO {
	    PODate?: string;
	    RequestedDeliveryDate?: string;
	    UUID: string;
	    State: string;
	    QuoteNumber: string;
	    CustomerName: string;
	    BillingAddress: string;
	    DeliveryAddress: string;
	    ShipToCode: string;
	    ContactEmail: string;
	    ContactPhone: string;
	    PONumber: string;
	    ReceivedDate: string;
	    Incoterms: string;
	    Currency: string;
	    TaxZone: string;
	    TaxCategory: string;
	    AdjustmentReason: string;
	    CreatedAt: string;
	    UpdatedAt: string;
	    Items: PurchaseOrderItemDTO[];
	    ID: number;
	    QuotationID: number;
	    SourceRevision: number;
	    CustomerID: number;
	    PaymentTerms: string;
	    DiscountPercent: number;
	    PpnBps: number;
	    SubtotalMinor: number;
	    DiscountMinor: number;
	    PpnMinor: number;
	    AdjustmentMinor: number;
	    GrandTotalMinor: number;
	    PrintedSubtotalMinor: number;
	    PrintedDiscountMinor: number;
	    PrintedPpnMinor: number;
	    PrintedGrandTotalMinor: number;
	    SavedAt?: string;
	    SavedBy?: number;
	    PdfAbsentConfirmed: boolean;
	    ConfirmationNumber?: string;
	    ConfirmationAssignedAt?: string;
	    ConfirmationWarn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PODate = source["PODate"];
	        this.RequestedDeliveryDate = source["RequestedDeliveryDate"];
	        this.UUID = source["UUID"];
	        this.State = source["State"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.BillingAddress = source["BillingAddress"];
	        this.DeliveryAddress = source["DeliveryAddress"];
	        this.ShipToCode = source["ShipToCode"];
	        this.ContactEmail = source["ContactEmail"];
	        this.ContactPhone = source["ContactPhone"];
	        this.PONumber = source["PONumber"];
	        this.ReceivedDate = source["ReceivedDate"];
	        this.Incoterms = source["Incoterms"];
	        this.Currency = source["Currency"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.AdjustmentReason = source["AdjustmentReason"];
	        this.CreatedAt = source["CreatedAt"];
	        this.UpdatedAt = source["UpdatedAt"];
	        this.Items = this.convertValues(source["Items"], PurchaseOrderItemDTO);
	        this.ID = source["ID"];
	        this.QuotationID = source["QuotationID"];
	        this.SourceRevision = source["SourceRevision"];
	        this.CustomerID = source["CustomerID"];
	        this.PaymentTerms = source["PaymentTerms"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.PpnBps = source["PpnBps"];
	        this.SubtotalMinor = source["SubtotalMinor"];
	        this.DiscountMinor = source["DiscountMinor"];
	        this.PpnMinor = source["PpnMinor"];
	        this.AdjustmentMinor = source["AdjustmentMinor"];
	        this.GrandTotalMinor = source["GrandTotalMinor"];
	        this.PrintedSubtotalMinor = source["PrintedSubtotalMinor"];
	        this.PrintedDiscountMinor = source["PrintedDiscountMinor"];
	        this.PrintedPpnMinor = source["PrintedPpnMinor"];
	        this.PrintedGrandTotalMinor = source["PrintedGrandTotalMinor"];
	        this.SavedAt = source["SavedAt"];
	        this.SavedBy = source["SavedBy"];
	        this.PdfAbsentConfirmed = source["PdfAbsentConfirmed"];
	        this.ConfirmationNumber = source["ConfirmationNumber"];
	        this.ConfirmationAssignedAt = source["ConfirmationAssignedAt"];
	        this.ConfirmationWarn = source["ConfirmationWarn"];
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
	export class PurchaseOrderDecisionInput {
	    Key: string;
	    Choice: string;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDecisionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Key = source["Key"];
	        this.Choice = source["Choice"];
	    }
	}
	export class PurchaseOrderDocumentContentDTO {
	    DataBase64: string;
	    ContentType: string;
	    Filename: string;
	    SHA256: string;
	    Version: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDocumentContentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DataBase64 = source["DataBase64"];
	        this.ContentType = source["ContentType"];
	        this.Filename = source["Filename"];
	        this.SHA256 = source["SHA256"];
	        this.Version = source["Version"];
	    }
	}
	export class PurchaseOrderDocumentDTO {
	    UUID: string;
	    SHA256: string;
	    ContentType: string;
	    OriginalFilename: string;
	    UploadedAt: string;
	    ReplacedAt?: string;
	    RemovedAt?: string;
	    RemovedBy?: number;
	    ID: number;
	    PurchaseOrderID: number;
	    Version: number;
	    SizeBytes: number;
	    UploadedBy: number;
	    Active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDocumentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.UUID = source["UUID"];
	        this.SHA256 = source["SHA256"];
	        this.ContentType = source["ContentType"];
	        this.OriginalFilename = source["OriginalFilename"];
	        this.UploadedAt = source["UploadedAt"];
	        this.ReplacedAt = source["ReplacedAt"];
	        this.RemovedAt = source["RemovedAt"];
	        this.RemovedBy = source["RemovedBy"];
	        this.ID = source["ID"];
	        this.PurchaseOrderID = source["PurchaseOrderID"];
	        this.Version = source["Version"];
	        this.SizeBytes = source["SizeBytes"];
	        this.UploadedBy = source["UploadedBy"];
	        this.Active = source["Active"];
	    }
	}
	export class PurchaseOrderDocumentStateDTO {
	    Document?: PurchaseOrderDocumentDTO;
	    Versions: PurchaseOrderDocumentDTO[];
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDocumentStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Document = this.convertValues(source["Document"], PurchaseOrderDocumentDTO);
	        this.Versions = this.convertValues(source["Versions"], PurchaseOrderDocumentDTO);
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
	export class PurchaseOrderDocumentUploadInput {
	    PurchaseOrderID: number;
	    DataBase64: string;
	    Filename: string;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderDocumentUploadInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PurchaseOrderID = source["PurchaseOrderID"];
	        this.DataBase64 = source["DataBase64"];
	        this.Filename = source["Filename"];
	    }
	}
	export class PurchaseOrderItemInput {
	    QuotationItemNo?: number;
	    QuotationSubNo?: number;
	    Description: string;
	    QuantityScaled: number;
	    UoM: string;
	    UnitPriceMinor: number;
	    DiscountEnabled: boolean;
	    DiscountPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuotationItemNo = source["QuotationItemNo"];
	        this.QuotationSubNo = source["QuotationSubNo"];
	        this.Description = source["Description"];
	        this.QuantityScaled = source["QuantityScaled"];
	        this.UoM = source["UoM"];
	        this.UnitPriceMinor = source["UnitPriceMinor"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.DiscountPercent = source["DiscountPercent"];
	    }
	}
	export class PurchaseOrderFinalizeInput {
	    ID: number;
	    BillingAddress: string;
	    DeliveryAddress: string;
	    ShipToCode: string;
	    ContactEmail: string;
	    ContactPhone: string;
	    PONumber: string;
	    PODate: string;
	    ReceivedDate: string;
	    RequestedDeliveryDate: string;
	    Incoterms: string;
	    PaymentTerms: string;
	    AdjustmentReason: string;
	    Items: PurchaseOrderItemInput[];
	    Decisions: PurchaseOrderDecisionInput[];
	    AcknowledgedMissing: string[];
	    AcknowledgedExtra: number[];
	    DiscountPercent: number;
	    AdjustmentMinor: number;
	    DocumentConfirmed: boolean;
	    AssignConfirmationNumber: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderFinalizeInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.BillingAddress = source["BillingAddress"];
	        this.DeliveryAddress = source["DeliveryAddress"];
	        this.ShipToCode = source["ShipToCode"];
	        this.ContactEmail = source["ContactEmail"];
	        this.ContactPhone = source["ContactPhone"];
	        this.PONumber = source["PONumber"];
	        this.PODate = source["PODate"];
	        this.ReceivedDate = source["ReceivedDate"];
	        this.RequestedDeliveryDate = source["RequestedDeliveryDate"];
	        this.Incoterms = source["Incoterms"];
	        this.PaymentTerms = source["PaymentTerms"];
	        this.AdjustmentReason = source["AdjustmentReason"];
	        this.Items = this.convertValues(source["Items"], PurchaseOrderItemInput);
	        this.Decisions = this.convertValues(source["Decisions"], PurchaseOrderDecisionInput);
	        this.AcknowledgedMissing = source["AcknowledgedMissing"];
	        this.AcknowledgedExtra = source["AcknowledgedExtra"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.AdjustmentMinor = source["AdjustmentMinor"];
	        this.DocumentConfirmed = source["DocumentConfirmed"];
	        this.AssignConfirmationNumber = source["AssignConfirmationNumber"];
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
	
	
	export class PurchaseOrderListCustomerDTO {
	    Name: string;
	    ID: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderListCustomerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.ID = source["ID"];
	    }
	}
	export class PurchaseOrderListRowDTO {
	    UUID: string;
	    State: string;
	    PONumber: string;
	    QuoteNumber: string;
	    CustomerName: string;
	    ReceivedDate: string;
	    Currency: string;
	    ID: number;
	    CustomerID: number;
	    SourceRevision: number;
	    SourceMissing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderListRowDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.UUID = source["UUID"];
	        this.State = source["State"];
	        this.PONumber = source["PONumber"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerName = source["CustomerName"];
	        this.ReceivedDate = source["ReceivedDate"];
	        this.Currency = source["Currency"];
	        this.ID = source["ID"];
	        this.CustomerID = source["CustomerID"];
	        this.SourceRevision = source["SourceRevision"];
	        this.SourceMissing = source["SourceMissing"];
	    }
	}
	export class PurchaseOrderListPageDTO {
	    PurchaseOrders: PurchaseOrderListRowDTO[];
	    Customers: PurchaseOrderListCustomerDTO[];
	    Page: number;
	    PageSize: number;
	    Total: number;
	    TotalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderListPageDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PurchaseOrders = this.convertValues(source["PurchaseOrders"], PurchaseOrderListRowDTO);
	        this.Customers = this.convertValues(source["Customers"], PurchaseOrderListCustomerDTO);
	        this.Page = source["Page"];
	        this.PageSize = source["PageSize"];
	        this.Total = source["Total"];
	        this.TotalPages = source["TotalPages"];
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
	export class PurchaseOrderListQuery {
	    Search: string;
	    State: string;
	    CustomerIDs: number[];
	    ReceivedFrom: string;
	    ReceivedTo: string;
	    SourceMissing: string;
	    Page: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderListQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Search = source["Search"];
	        this.State = source["State"];
	        this.CustomerIDs = source["CustomerIDs"];
	        this.ReceivedFrom = source["ReceivedFrom"];
	        this.ReceivedTo = source["ReceivedTo"];
	        this.SourceMissing = source["SourceMissing"];
	        this.Page = source["Page"];
	    }
	}
	
	export class PurchaseOrderSourceLineDTO {
	    QuotationItemNo: number;
	    QuotationSubNo?: number;
	    LineLabel: string;
	    Description: string;
	    UoM: string;
	    QuantityScaled: number;
	    UnitPriceMinor: number;
	    DiscountEnabled: boolean;
	    DiscountPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderSourceLineDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuotationItemNo = source["QuotationItemNo"];
	        this.QuotationSubNo = source["QuotationSubNo"];
	        this.LineLabel = source["LineLabel"];
	        this.Description = source["Description"];
	        this.UoM = source["UoM"];
	        this.QuantityScaled = source["QuantityScaled"];
	        this.UnitPriceMinor = source["UnitPriceMinor"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.DiscountPercent = source["DiscountPercent"];
	    }
	}
	export class PurchaseOrderSourceDTO {
	    QuotationID: number;
	    RevisionNumber: number;
	    QuoteNumber: string;
	    CustomerID: number;
	    CustomerName: string;
	    Currency: string;
	    PaymentTerms: string;
	    Incoterms: string;
	    DiscountPercent: number;
	    Lines: PurchaseOrderSourceLineDTO[];
	    PrintedSubtotalMinor: number;
	    PrintedDiscountMinor: number;
	    PrintedPpnMinor: number;
	    PrintedGrandTotalMinor: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderSourceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuotationID = source["QuotationID"];
	        this.RevisionNumber = source["RevisionNumber"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.CustomerID = source["CustomerID"];
	        this.CustomerName = source["CustomerName"];
	        this.Currency = source["Currency"];
	        this.PaymentTerms = source["PaymentTerms"];
	        this.Incoterms = source["Incoterms"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.Lines = this.convertValues(source["Lines"], PurchaseOrderSourceLineDTO);
	        this.PrintedSubtotalMinor = source["PrintedSubtotalMinor"];
	        this.PrintedDiscountMinor = source["PrintedDiscountMinor"];
	        this.PrintedPpnMinor = source["PrintedPpnMinor"];
	        this.PrintedGrandTotalMinor = source["PrintedGrandTotalMinor"];
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
	export class PurchaseOrderReviewDTO {
	    Order: PurchaseOrderDTO;
	    Source: PurchaseOrderSourceDTO;
	    DocumentState: string;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderReviewDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Order = this.convertValues(source["Order"], PurchaseOrderDTO);
	        this.Source = this.convertValues(source["Source"], PurchaseOrderSourceDTO);
	        this.DocumentState = source["DocumentState"];
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
	
	
	export class PurchaseOrderUpdateInput {
	    ID: number;
	    BillingAddress: string;
	    DeliveryAddress: string;
	    ShipToCode: string;
	    ContactEmail: string;
	    ContactPhone: string;
	    PONumber: string;
	    PODate: string;
	    ReceivedDate: string;
	    RequestedDeliveryDate: string;
	    Incoterms: string;
	    PaymentTerms: string;
	    AdjustmentReason: string;
	    Items: PurchaseOrderItemInput[];
	    DiscountPercent: number;
	    AdjustmentMinor: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseOrderUpdateInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.BillingAddress = source["BillingAddress"];
	        this.DeliveryAddress = source["DeliveryAddress"];
	        this.ShipToCode = source["ShipToCode"];
	        this.ContactEmail = source["ContactEmail"];
	        this.ContactPhone = source["ContactPhone"];
	        this.PONumber = source["PONumber"];
	        this.PODate = source["PODate"];
	        this.ReceivedDate = source["ReceivedDate"];
	        this.RequestedDeliveryDate = source["RequestedDeliveryDate"];
	        this.Incoterms = source["Incoterms"];
	        this.PaymentTerms = source["PaymentTerms"];
	        this.AdjustmentReason = source["AdjustmentReason"];
	        this.Items = this.convertValues(source["Items"], PurchaseOrderItemInput);
	        this.DiscountPercent = source["DiscountPercent"];
	        this.AdjustmentMinor = source["AdjustmentMinor"];
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
	export class QuotationSubSubItemInput {
	    Description: string;
	
	    static createFrom(source: any = {}) {
	        return new QuotationSubSubItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	    }
	}
	export class QuotationSubItemInput {
	    Description: string;
	    UoM: string;
	    UnitCost: number;
	    Qty: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    DiscountEnabled: boolean;
	    MarginEnabled: boolean;
	    SubSubItems: QuotationSubSubItemInput[];
	
	    static createFrom(source: any = {}) {
	        return new QuotationSubItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.UoM = source["UoM"];
	        this.UnitCost = source["UnitCost"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.SubSubItems = this.convertValues(source["SubSubItems"], QuotationSubSubItemInput);
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
	    UoM: string;
	    SubItems: QuotationSubItemInput[];
	    UnitPrice: number;
	    Qty: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    DiscountEnabled: boolean;
	    MarginEnabled: boolean;
	    ShowSubPrices: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QuotationItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.UoM = source["UoM"];
	        this.SubItems = this.convertValues(source["SubItems"], QuotationSubItemInput);
	        this.UnitPrice = source["UnitPrice"];
	        this.Qty = source["Qty"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.ShowSubPrices = source["ShowSubPrices"];
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
	export class QuotationInput {
	    Incoterm: string;
	    Subject: string;
	    RecipientName: string;
	    TaxZone: string;
	    TaxCategory: string;
	    TaxType: string;
	    ServiceCategory: string;
	    Items: QuotationItemInput[];
	    DiscountPercent: number;
	    PaymentTerm: number;
	    LeadTimeYears: number;
	    LeadTimeMonths: number;
	    LeadTimeWeeks: number;
	    LeadTimeDays: number;
	    CustomerID: number;
	    Validity: number;
	
	    static createFrom(source: any = {}) {
	        return new QuotationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Incoterm = source["Incoterm"];
	        this.Subject = source["Subject"];
	        this.RecipientName = source["RecipientName"];
	        this.TaxZone = source["TaxZone"];
	        this.TaxCategory = source["TaxCategory"];
	        this.TaxType = source["TaxType"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.Items = this.convertValues(source["Items"], QuotationItemInput);
	        this.DiscountPercent = source["DiscountPercent"];
	        this.PaymentTerm = source["PaymentTerm"];
	        this.LeadTimeYears = source["LeadTimeYears"];
	        this.LeadTimeMonths = source["LeadTimeMonths"];
	        this.LeadTimeWeeks = source["LeadTimeWeeks"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.CustomerID = source["CustomerID"];
	        this.Validity = source["Validity"];
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
	export class SavedDraft {
	    Payload: string;
	    Subject: string;
	    CustomerName: string;
	    Label: string;
	    ID: number;
	    ItemCount: number;
	    CustomerID: number;
	
	    static createFrom(source: any = {}) {
	        return new SavedDraft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Payload = source["Payload"];
	        this.Subject = source["Subject"];
	        this.CustomerName = source["CustomerName"];
	        this.Label = source["Label"];
	        this.ID = source["ID"];
	        this.ItemCount = source["ItemCount"];
	        this.CustomerID = source["CustomerID"];
	    }
	}
	export class UpdateInfo {
	    Current: string;
	    Latest: string;
	    NotesURL: string;
	    DownloadURL: string;
	    UpdateAvailable: boolean;
	    Required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Current = source["Current"];
	        this.Latest = source["Latest"];
	        this.NotesURL = source["NotesURL"];
	        this.DownloadURL = source["DownloadURL"];
	        this.UpdateAvailable = source["UpdateAvailable"];
	        this.Required = source["Required"];
	    }
	}

}

export namespace currency {
	
	export class CurrencyInfo {
	    code: string;
	    exponent: number;
	    symbol: string;
	    position: string;
	    groupSep: string;
	    decimalSep: string;
	    symbolSpaced: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CurrencyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.exponent = source["exponent"];
	        this.symbol = source["symbol"];
	        this.position = source["position"];
	        this.groupSep = source["groupSep"];
	        this.decimalSep = source["decimalSep"];
	        this.symbolSpaced = source["symbolSpaced"];
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
	export class RevisionSubSubItem {
	    SubSubNo: number;
	    Description: string;
	
	    static createFrom(source: any = {}) {
	        return new RevisionSubSubItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SubSubNo = source["SubSubNo"];
	        this.Description = source["Description"];
	    }
	}
	export class RevisionSubItem {
	    SubNo: number;
	    Description: string;
	    UoM: string;
	    UnitCost: number;
	    UnitPrice: number;
	    MarginAmount: number;
	    Total: number;
	    Qty: number;
	    ListUnitPrice: number;
	    DiscountAmount: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	    SubSubItems: RevisionSubSubItem[];
	
	    static createFrom(source: any = {}) {
	        return new RevisionSubItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SubNo = source["SubNo"];
	        this.Description = source["Description"];
	        this.UoM = source["UoM"];
	        this.UnitCost = source["UnitCost"];
	        this.UnitPrice = source["UnitPrice"];
	        this.MarginAmount = source["MarginAmount"];
	        this.Total = source["Total"];
	        this.Qty = source["Qty"];
	        this.ListUnitPrice = source["ListUnitPrice"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.SubSubItems = this.convertValues(source["SubSubItems"], RevisionSubSubItem);
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
	export class RevisionItem {
	    Description: string;
	    UoM: string;
	    ItemNo: number;
	    Qty: number;
	    UnitPrice: number;
	    MarginAmount: number;
	    OriginalTotal: number;
	    MarginalizedUnitPrice: number;
	    MarginalizedTotal: number;
	    ListUnitPrice: number;
	    DiscountAmount: number;
	    MarginPercent: number;
	    DiscountPercent: number;
	    MarginEnabled: boolean;
	    DiscountEnabled: boolean;
	    ShowSubPrices: boolean;
	    SubItems: RevisionSubItem[];
	
	    static createFrom(source: any = {}) {
	        return new RevisionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Description = source["Description"];
	        this.UoM = source["UoM"];
	        this.ItemNo = source["ItemNo"];
	        this.Qty = source["Qty"];
	        this.UnitPrice = source["UnitPrice"];
	        this.MarginAmount = source["MarginAmount"];
	        this.OriginalTotal = source["OriginalTotal"];
	        this.MarginalizedUnitPrice = source["MarginalizedUnitPrice"];
	        this.MarginalizedTotal = source["MarginalizedTotal"];
	        this.ListUnitPrice = source["ListUnitPrice"];
	        this.DiscountAmount = source["DiscountAmount"];
	        this.MarginPercent = source["MarginPercent"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.MarginEnabled = source["MarginEnabled"];
	        this.DiscountEnabled = source["DiscountEnabled"];
	        this.ShowSubPrices = source["ShowSubPrices"];
	        this.SubItems = this.convertValues(source["SubItems"], RevisionSubItem);
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
	export class RevisionDraft {
	    TaxCategory: string;
	    PDFPath: string;
	    CustomerName: string;
	    CustomerAddress: string;
	    QuotationUUID: string;
	    RecipientName: string;
	    RecipientAddress: string;
	    Subject: string;
	    Incoterm: string;
	    IssuerName: string;
	    Currency: string;
	    CanonicalSHA256: string;
	    SigKeyID: string;
	    Signature: string;
	    TaxZone: string;
	    DocumentHash: string;
	    ServiceCategory: string;
	    QuoteNumber: string;
	    HTMLSnapshot: string;
	    TaxType: string;
	    Items: RevisionItem[];
	    PDFData: number[];
	    Totals: RevisionTotals;
	    TaxRateBps: number;
	    CustomerCode: number;
	    DiscountPercent: number;
	    LeadTimeDays: number;
	    LeadTimeYears: number;
	    LeadTimeMonths: number;
	    LeadTimeWeeks: number;
	    PaymentTermDays: number;
	    ValidityDays: number;
	    RevisionNumber: number;
	
	    static createFrom(source: any = {}) {
	        return new RevisionDraft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TaxCategory = source["TaxCategory"];
	        this.PDFPath = source["PDFPath"];
	        this.CustomerName = source["CustomerName"];
	        this.CustomerAddress = source["CustomerAddress"];
	        this.QuotationUUID = source["QuotationUUID"];
	        this.RecipientName = source["RecipientName"];
	        this.RecipientAddress = source["RecipientAddress"];
	        this.Subject = source["Subject"];
	        this.Incoterm = source["Incoterm"];
	        this.IssuerName = source["IssuerName"];
	        this.Currency = source["Currency"];
	        this.CanonicalSHA256 = source["CanonicalSHA256"];
	        this.SigKeyID = source["SigKeyID"];
	        this.Signature = source["Signature"];
	        this.TaxZone = source["TaxZone"];
	        this.DocumentHash = source["DocumentHash"];
	        this.ServiceCategory = source["ServiceCategory"];
	        this.QuoteNumber = source["QuoteNumber"];
	        this.HTMLSnapshot = source["HTMLSnapshot"];
	        this.TaxType = source["TaxType"];
	        this.Items = this.convertValues(source["Items"], RevisionItem);
	        this.PDFData = source["PDFData"];
	        this.Totals = this.convertValues(source["Totals"], RevisionTotals);
	        this.TaxRateBps = source["TaxRateBps"];
	        this.CustomerCode = source["CustomerCode"];
	        this.DiscountPercent = source["DiscountPercent"];
	        this.LeadTimeDays = source["LeadTimeDays"];
	        this.LeadTimeYears = source["LeadTimeYears"];
	        this.LeadTimeMonths = source["LeadTimeMonths"];
	        this.LeadTimeWeeks = source["LeadTimeWeeks"];
	        this.PaymentTermDays = source["PaymentTermDays"];
	        this.ValidityDays = source["ValidityDays"];
	        this.RevisionNumber = source["RevisionNumber"];
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

