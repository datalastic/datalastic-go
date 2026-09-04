package datalastic

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// VesselsResource groups vessel tracking and lookup endpoints.
type VesselsResource struct {
	client *Client
}

// VesselParams identifies a single vessel by UUID, MMSI, or IMO.
type VesselParams struct {
	UUID string
	MMSI string
	IMO  string
}

func (p VesselParams) values() (url.Values, error) {
	v := url.Values{}
	addString(v, "uuid", p.UUID)
	addString(v, "mmsi", p.MMSI)
	addString(v, "imo", p.IMO)
	if len(v) == 0 {
		return nil, &DatalasticError{Message: "one of uuid, mmsi, or imo is required"}
	}
	return v, nil
}

// VesselBulkParams selects multiple vessels by repeated identifiers.
type VesselBulkParams struct {
	MMSI []string
	IMO  []string
	UUID []string
}

// VesselInRadiusParams scans for vessels within a radius of a point or port.
type VesselInRadiusParams struct {
	Lat          *float64
	Lon          *float64
	PortUUID     string
	PortUnlocode string
	UUID         string
	MMSI         string
	IMO          string
	Radius       float64 // required
	Type         string
	TypeSpecific string
	Exclude      string
	NavStatus    *int
	// IncludeNullType maps to the _empty_ param: include/exclude null-type
	// vessels.
	IncludeNullType *bool
	Next            string
}

// VesselHistoryParams requests historical positions for a vessel.
type VesselHistoryParams struct {
	UUID string
	MMSI string
	IMO  string
	Days *int
	From string
	To   string
}

// VesselFindParams searches the static vessel database.
type VesselFindParams struct {
	Name            string
	Fuzzy           *int
	VesselType      string // maps to "type" query param
	TypeSpecific    string
	CountryISO      string
	GrossTonnageMin *int
	GrossTonnageMax *int
	DeadweightMin   *int
	DeadweightMax   *int
	LengthMin       *float64
	LengthMax       *float64
	BreadthMin      *float64
	BreadthMax      *float64
	YearBuiltMin    *int
	YearBuiltMax    *int
	// IncludeNullType maps to the _empty_ param: include/exclude null-type
	// vessels. Counts as a search filter, like VesselType.
	IncludeNullType *bool
	Next            string
}

// Get returns basic AIS tracking data for a vessel.
func (r *VesselsResource) Get(p VesselParams) (*Vessel, error) {
	v, err := p.values()
	if err != nil {
		return nil, err
	}
	raw, meta, err := r.client.do(r.client.baseV0, "vessel", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[Vessel](raw, "vessel", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// Pro returns extended tracking data including ETA, ATD, and port info.
func (r *VesselsResource) Pro(p VesselParams) (*VesselPro, error) {
	v, err := p.values()
	if err != nil {
		return nil, err
	}
	raw, meta, err := r.client.do(r.client.baseV0, "vessel_pro", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[VesselPro](raw, "vessel_pro", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// Bulk returns tracking data for multiple vessels in one request.
func (r *VesselsResource) Bulk(p VesselBulkParams) (*VesselBulkResult, error) {
	v := url.Values{}
	for _, m := range p.MMSI {
		if m != "" {
			v.Add("mmsi", m)
		}
	}
	for _, i := range p.IMO {
		if i != "" {
			v.Add("imo", i)
		}
	}
	for _, u := range p.UUID {
		if u != "" {
			v.Add("uuid", u)
		}
	}
	if len(v) == 0 {
		return nil, &DatalasticError{Message: "at least one mmsi, imo, or uuid is required"}
	}
	raw, meta, err := r.client.do(r.client.baseV0, "vessel_bulk", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[VesselBulkResult](raw, "vessel_bulk", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// InRadius scans for vessels within a radius of a coordinate or port.
func (r *VesselsResource) InRadius(p VesselInRadiusParams) (*VesselInRadiusResult, error) {
	if p.Radius <= 0 {
		return nil, &DatalasticError{Message: "radius is required and must be greater than zero"}
	}
	hasCenter := (p.Lat != nil && p.Lon != nil) || p.PortUUID != "" || p.PortUnlocode != "" ||
		p.UUID != "" || p.MMSI != "" || p.IMO != ""
	if !hasCenter {
		return nil, &DatalasticError{Message: "a center is required: provide lat/lon, a port, or a vessel identifier"}
	}

	v := url.Values{}
	addOptionalFloat(v, "lat", p.Lat)
	addOptionalFloat(v, "lon", p.Lon)
	addString(v, "port_uuid", p.PortUUID)
	addString(v, "port_unlocode", p.PortUnlocode)
	addString(v, "uuid", p.UUID)
	addString(v, "mmsi", p.MMSI)
	addString(v, "imo", p.IMO)
	v.Set("radius", strconvFloat(p.Radius))
	addString(v, "type", p.Type)
	addString(v, "type_specific", p.TypeSpecific)
	addString(v, "exclude", p.Exclude)
	addOptionalInt(v, "nav_status", p.NavStatus)
	if p.IncludeNullType != nil {
		if *p.IncludeNullType {
			v.Set("_empty_", "true")
		} else {
			v.Set("_empty_", "false")
		}
	}
	addString(v, "next", p.Next)

	raw, meta, err := r.client.do(r.client.baseV0, "vessel_inradius", v)
	if err != nil {
		return nil, err
	}
	result, err := decodeInto[VesselInRadiusResult](raw, "vessel_inradius", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	result.Meta = meta
	result.Next = meta.Next
	return result, nil
}

// History returns historical positions for a vessel.
func (r *VesselsResource) History(p VesselHistoryParams) (*VesselHistory, error) {
	v := url.Values{}
	addString(v, "uuid", p.UUID)
	addString(v, "mmsi", p.MMSI)
	addString(v, "imo", p.IMO)
	if len(v) == 0 {
		return nil, &DatalasticError{Message: "one of uuid, mmsi, or imo is required"}
	}
	addOptionalInt(v, "days", p.Days)
	addString(v, "from", p.From)
	addString(v, "to", p.To)

	raw, meta, err := r.client.do(r.client.baseV0, "vessel_history", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[VesselHistory](raw, "vessel_history", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// Info returns static vessel specifications.
func (r *VesselsResource) Info(p VesselParams) (*VesselInfo, error) {
	v, err := p.values()
	if err != nil {
		return nil, err
	}
	raw, meta, err := r.client.do(r.client.baseV0, "vessel_info", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[VesselInfo](raw, "vessel_info", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// Find searches the static vessel database. At least one of Name, VesselType,
// TypeSpecific, CountryISO, IncludeNullType, or a range bound is required;
// Fuzzy and Next alone are not sufficient.
func (r *VesselsResource) Find(p VesselFindParams) (*VesselFindResult, error) {
	v := url.Values{}
	addString(v, "name", p.Name)
	addOptionalInt(v, "fuzzy", p.Fuzzy)
	addString(v, "next", p.Next)

	searchCount := 0
	if p.Name != "" {
		searchCount++
	}
	if p.IncludeNullType != nil {
		if *p.IncludeNullType {
			v.Set("_empty_", "true")
		} else {
			v.Set("_empty_", "false")
		}
		searchCount++
	}
	if p.VesselType != "" {
		v.Set("type", p.VesselType)
		searchCount++
	}
	if p.TypeSpecific != "" {
		v.Set("type_specific", p.TypeSpecific)
		searchCount++
	}
	if p.CountryISO != "" {
		v.Set("country_iso", p.CountryISO)
		searchCount++
	}
	searchCount += setIntRange(v, "gross_tonnage_min", p.GrossTonnageMin, "gross_tonnage_max", p.GrossTonnageMax)
	searchCount += setIntRange(v, "deadweight_min", p.DeadweightMin, "deadweight_max", p.DeadweightMax)
	searchCount += setFloatRange(v, "length_min", p.LengthMin, "length_max", p.LengthMax)
	searchCount += setFloatRange(v, "breadth_min", p.BreadthMin, "breadth_max", p.BreadthMax)
	searchCount += setIntRange(v, "year_built_min", p.YearBuiltMin, "year_built_max", p.YearBuiltMax)

	if searchCount == 0 {
		return nil, &DatalasticError{Message: "at least one search parameter is required (fuzzy and next do not count)"}
	}

	raw, meta, err := r.client.do(r.client.baseV0, "vessel_find", v)
	if err != nil {
		return nil, err
	}
	items, err := decodeSlice[VesselInfo](raw, "vessel_find", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	return &VesselFindResult{Meta: meta, Items: items, Next: meta.Next}, nil
}

// Estimated returns a satellite-estimated position for a vessel. Uses the
// extended base URL.
func (r *VesselsResource) Estimated(p VesselParams) (*VesselEstimated, error) {
	v, err := p.values()
	if err != nil {
		return nil, err
	}
	raw, meta, err := r.client.do(r.client.baseExt, "vessel_pro_est", v)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[VesselEstimated](raw, "vessel_pro_est", r.client.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// --- decode helpers ---
//
// Both helpers take the API key so a decoder error, which can quote the payload
// it failed on, is redacted like every other error the SDK returns. They stay
// free functions because Go methods cannot be generic.

func decodeInto[T any](raw json.RawMessage, ctx, apiKey string) (*T, error) {
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to decode %s: %v", ctx, redactKey(err.Error(), apiKey))}}
	}
	return &out, nil
}

func decodeSlice[T any](raw json.RawMessage, ctx, apiKey string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to decode %s: %v", ctx, redactKey(err.Error(), apiKey))}}
	}
	return out, nil
}

func setIntRange(v url.Values, minKey string, min *int, maxKey string, max *int) int {
	count := 0
	if min != nil {
		addOptionalInt(v, minKey, min)
		count++
	}
	if max != nil {
		addOptionalInt(v, maxKey, max)
		count++
	}
	return count
}

func setFloatRange(v url.Values, minKey string, min *float64, maxKey string, max *float64) int {
	count := 0
	if min != nil {
		addOptionalFloat(v, minKey, min)
		count++
	}
	if max != nil {
		addOptionalFloat(v, maxKey, max)
		count++
	}
	return count
}
