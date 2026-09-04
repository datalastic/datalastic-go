package datalastic

import (
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

// DryDock returns dry-dock and survey records. It is DryDockWithMeta without
// the response metadata.
func (r *IntelResource) DryDock(p IntelDryDockParams) ([]DryDockRecord, error) {
	records, _, err := r.DryDockWithMeta(p)
	return records, err
}

// DryDockWithMeta is DryDock, additionally returning the response envelope
// metadata.
func (r *IntelResource) DryDockWithMeta(p IntelDryDockParams) ([]DryDockRecord, *Meta, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "dry_dock_from", p.DryDockFrom)
	addString(v, "dry_dock_to", p.DryDockTo)
	return intelSlice[DryDockRecord](r, "dry_dock_dates", v)
}

// Casualties returns casualty records. It is CasualtiesWithMeta without the
// response metadata.
func (r *IntelResource) Casualties(p IntelDateRangeParams) ([]CasualtyRecord, error) {
	records, _, err := r.CasualtiesWithMeta(p)
	return records, err
}

// CasualtiesWithMeta is Casualties, additionally returning the response
// envelope metadata.
func (r *IntelResource) CasualtiesWithMeta(p IntelDateRangeParams) ([]CasualtyRecord, *Meta, error) {
	return intelSlice[CasualtyRecord](r, "casualty", dateRangeValues(p))
}

// Inspections returns port-state-control inspection records. It is
// InspectionsWithMeta without the response metadata.
func (r *IntelResource) Inspections(p IntelDateRangeParams) ([]InspectionRecord, error) {
	records, _, err := r.InspectionsWithMeta(p)
	return records, err
}

// InspectionsWithMeta is Inspections, additionally returning the response
// envelope metadata.
func (r *IntelResource) InspectionsWithMeta(p IntelDateRangeParams) ([]InspectionRecord, *Meta, error) {
	return intelSlice[InspectionRecord](r, "inspections", dateRangeValues(p))
}

// SPD returns sales, purchase, and demolition records. It is SPDWithMeta
// without the response metadata.
func (r *IntelResource) SPD(p IntelDateRangeParams) ([]SPDRecord, error) {
	records, _, err := r.SPDWithMeta(p)
	return records, err
}

// SPDWithMeta is SPD, additionally returning the response envelope metadata.
func (r *IntelResource) SPDWithMeta(p IntelDateRangeParams) ([]SPDRecord, *Meta, error) {
	return intelSlice[SPDRecord](r, "spd", dateRangeValues(p))
}

// Ownership returns beneficial owner and management records. It is
// OwnershipWithMeta without the response metadata.
func (r *IntelResource) Ownership(p OwnershipParams) ([]OwnershipRecord, error) {
	records, _, err := r.OwnershipWithMeta(p)
	return records, err
}

// OwnershipWithMeta is Ownership, additionally returning the response envelope
// metadata.
func (r *IntelResource) OwnershipWithMeta(p OwnershipParams) ([]OwnershipRecord, *Meta, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "beneficial_owner", p.BeneficialOwner)
	addString(v, "operator", p.Operator)
	addString(v, "technical_manager", p.TechnicalManager)
	addString(v, "commercial_manager", p.CommercialManager)
	addString(v, "updated_from", p.UpdatedFrom)
	return intelSlice[OwnershipRecord](r, "ownership", v)
}

// ClassSociety returns classification society records. It is
// ClassSocietyWithMeta without the response metadata.
func (r *IntelResource) ClassSociety(p ClassSocietyParams) ([]ClassSocietyRecord, error) {
	records, _, err := r.ClassSocietyWithMeta(p)
	return records, err
}

// ClassSocietyWithMeta is ClassSociety, additionally returning the response
// envelope metadata.
func (r *IntelResource) ClassSocietyWithMeta(p ClassSocietyParams) ([]ClassSocietyRecord, *Meta, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addOptionalInt(v, "fuzzy", p.Fuzzy)
	addString(v, "beneficial_owner", p.BeneficialOwner)
	addString(v, "beneficial_owner_imo", p.BeneficialOwnerIMO)
	addString(v, "technical_manager", p.TechnicalManager)
	addString(v, "technical_manager_imo", p.TechnicalManagerIMO)
	addString(v, "updated_from", p.UpdatedFrom)
	return intelSlice[ClassSocietyRecord](r, "class_society", v)
}

// Engine returns engine and propulsion records. It is EngineWithMeta without
// the response metadata.
func (r *IntelResource) Engine(p EngineParams) ([]EngineRecord, error) {
	records, _, err := r.EngineWithMeta(p)
	return records, err
}

// EngineWithMeta is Engine, additionally returning the response envelope
// metadata.
func (r *IntelResource) EngineWithMeta(p EngineParams) ([]EngineRecord, *Meta, error) {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addOptionalInt(v, "fuzzy", p.Fuzzy)
	addString(v, "updated_from", p.UpdatedFrom)
	return intelSlice[EngineRecord](r, "engine", v)
}

// Companies returns maritime company records. It is CompaniesWithMeta without
// the response metadata.
func (r *IntelResource) Companies(p CompanyParams) ([]CompanyRecord, error) {
	records, _, err := r.CompaniesWithMeta(p)
	return records, err
}

// CompaniesWithMeta is Companies, additionally returning the response envelope
// metadata.
func (r *IntelResource) CompaniesWithMeta(p CompanyParams) ([]CompanyRecord, *Meta, error) {
	v := url.Values{}
	addString(v, "company_imo", p.CompanyIMO)
	addString(v, "name", p.Name)
	addString(v, "updated_from", p.UpdatedFrom)
	return intelSlice[CompanyRecord](r, "companies", v)
}

// dateRangeValues builds the shared query for vessel + from/to endpoints.
func dateRangeValues(p IntelDateRangeParams) url.Values {
	v := url.Values{}
	addString(v, "imo", p.IMO)
	addString(v, "name", p.Name)
	addString(v, "from", p.From)
	addString(v, "to", p.To)
	return v
}

// intelSlice performs a maritime-reports GET and decodes the payload as a slice
// of T, keeping the response envelope metadata.
func intelSlice[T any](r *IntelResource, path string, v url.Values) ([]T, *Meta, error) {
	raw, meta, err := r.client.do(r.client.baseMR, path, v)
	if err != nil {
		return nil, nil, err
	}
	records, err := decodeSlice[T](raw, path, r.client.apiKey)
	if err != nil {
		return nil, nil, err
	}
	return records, meta, nil
}
