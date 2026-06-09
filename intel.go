package datalastic

import (
	"encoding/json"
	"net/url"
)

// IntelResource groups maritime intelligence report endpoints. All requests use
// the maritime_reports base URL.
type IntelResource struct {
	client *Client
}

// IntelVesselParams identifies a vessel by IMO or name.
type IntelVesselParams struct {
	IMO  string
	Name string
}

// IntelDryDockParams filters dry-dock records by vessel and dry-dock date range.
type IntelDryDockParams struct {
	IMO         string
	Name        string
	DryDockFrom string
	DryDockTo   string
}

// IntelDateRangeParams filters records by vessel and a generic date range.
type IntelDateRangeParams struct {
	IMO  string
	Name string
	From string
	To   string
}

// OwnershipParams filters ownership records.
type OwnershipParams struct {
	IMO               string
	Name              string
	BeneficialOwner   string
	Operator          string
	TechnicalManager  string
	CommercialManager string
	UpdatedFrom       string
}

// ClassSocietyParams filters classification society records.
type ClassSocietyParams struct {
	IMO                 string
	Name                string
	Fuzzy               *int
	BeneficialOwner     string
	BeneficialOwnerIMO  string
	TechnicalManager    string
	TechnicalManagerIMO string
	UpdatedFrom         string
}

// EngineParams filters engine records.
type EngineParams struct {
	IMO         string
	Name        string
	Fuzzy       *int
	UpdatedFrom string
}

// CompanyParams filters company records.
type CompanyParams struct {
	CompanyIMO  string
	Name        string
	UpdatedFrom string
}

// DryDock returns dry-dock and survey records.
func (r *IntelResource) DryDock(p IntelDryDockParams) ([]DryDockRecord, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "dry_dock_from", p.DryDockFrom)
	addString(v, "dry_dock_to", p.DryDockTo)
	raw, err := r.client.do(r.client.baseMR, "dry_dock_dates", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[DryDockRecord](raw, "dry_dock_dates")
}

// Casualties returns casualty records.
func (r *IntelResource) Casualties(p IntelDateRangeParams) ([]CasualtyRecord, error) {
	raw, err := r.dateRange("casualty", p)
	if err != nil {
		return nil, err
	}
	return decodeSlice[CasualtyRecord](raw, "casualty")
}

// Inspections returns port-state-control inspection records.
func (r *IntelResource) Inspections(p IntelDateRangeParams) ([]InspectionRecord, error) {
	raw, err := r.dateRange("inspections", p)
	if err != nil {
		return nil, err
	}
	return decodeSlice[InspectionRecord](raw, "inspections")
}

// SPD returns sales, purchase, and demolition records.
func (r *IntelResource) SPD(p IntelDateRangeParams) ([]SPDRecord, error) {
	raw, err := r.dateRange("spd", p)
	if err != nil {
		return nil, err
	}
	return decodeSlice[SPDRecord](raw, "spd")
}

// Ownership returns beneficial owner and management records.
func (r *IntelResource) Ownership(p OwnershipParams) ([]OwnershipRecord, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "beneficial_owner", p.BeneficialOwner)
	addString(v, "operator", p.Operator)
	addString(v, "technical_manager", p.TechnicalManager)
	addString(v, "commercial_manager", p.CommercialManager)
	addString(v, "updated_from", p.UpdatedFrom)
	raw, err := r.client.do(r.client.baseMR, "ownership", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[OwnershipRecord](raw, "ownership")
}

// ClassSociety returns classification society records.
func (r *IntelResource) ClassSociety(p ClassSocietyParams) ([]ClassSocietyRecord, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addOptionalInt(v, "fuzzy", p.Fuzzy)
	addString(v, "beneficial_owner", p.BeneficialOwner)
	addString(v, "beneficial_owner_imo", p.BeneficialOwnerIMO)
	addString(v, "technical_manager", p.TechnicalManager)
	addString(v, "technical_manager_imo", p.TechnicalManagerIMO)
	addString(v, "updated_from", p.UpdatedFrom)
	raw, err := r.client.do(r.client.baseMR, "class_society", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[ClassSocietyRecord](raw, "class_society")
}

// Engine returns engine and propulsion records.
func (r *IntelResource) Engine(p EngineParams) ([]EngineRecord, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addOptionalInt(v, "fuzzy", p.Fuzzy)
	addString(v, "updated_from", p.UpdatedFrom)
	raw, err := r.client.do(r.client.baseMR, "engine", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[EngineRecord](raw, "engine")
}

// Companies returns maritime company records.
func (r *IntelResource) Companies(p CompanyParams) ([]CompanyRecord, error) {
	v := url.Values{}
	addString(v, "company_imo", p.CompanyIMO)
	addString(v, "name", p.Name)
	addString(v, "updated_from", p.UpdatedFrom)
	raw, err := r.client.do(r.client.baseMR, "companies", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[CompanyRecord](raw, "companies")
}

// dateRange builds the shared query for vessel + from/to endpoints.
func (r *IntelResource) dateRange(path string, p IntelDateRangeParams) (json.RawMessage, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "from", p.From)
	addString(v, "to", p.To)
	return r.client.do(r.client.baseMR, path, v)
}
