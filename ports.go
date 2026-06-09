package datalastic

import (
	"net/url"
)

// PortsResource groups port lookup endpoints.
type PortsResource struct {
	client *Client
}

// PortFindParams searches the port database.
type PortFindParams struct {
	Name       string
	UUID       string
	Fuzzy      *int
	PortType   string
	CountryISO string
	Unlocode   string
	Lat        *float64
	Lon        *float64
	Radius     *float64
}

// PortGetParams identifies a single port, including its terminals.
type PortGetParams struct {
	Name     string
	UUID     string
	Unlocode string
	Lat      *float64
	Lon      *float64
	Radius   *float64
}

// Find searches the port database. At least one search parameter is required.
func (r *PortsResource) Find(p PortFindParams) ([]Port, error) {
	v := url.Values{}
	addOptionalInt(v, "fuzzy", p.Fuzzy)

	searchCount := 0
	if p.Name != "" {
		v.Set("name", p.Name)
		searchCount++
	}
	if p.UUID != "" {
		v.Set("uuid", p.UUID)
		searchCount++
	}
	if p.PortType != "" {
		v.Set("port_type", p.PortType)
		searchCount++
	}
	if p.CountryISO != "" {
		v.Set("country_iso", p.CountryISO)
		searchCount++
	}
	if p.Unlocode != "" {
		v.Set("unlocode", p.Unlocode)
		searchCount++
	}
	if p.Lat != nil && p.Lon != nil {
		addOptionalFloat(v, "lat", p.Lat)
		addOptionalFloat(v, "lon", p.Lon)
		addOptionalFloat(v, "radius", p.Radius)
		searchCount++
	}

	if searchCount == 0 {
		return nil, &DatalasticError{Message: "at least one search parameter is required (fuzzy and radius do not count)"}
	}

	raw, err := r.client.do(r.client.baseV0, "port_find", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[Port](raw, "port_find")
}

// Get returns full detail for a single port, including terminals.
func (r *PortsResource) Get(p PortGetParams) (*PortDetail, error) {
	v := url.Values{}
	hasIdentifier := p.Name != "" || p.UUID != "" || p.Unlocode != "" || (p.Lat != nil && p.Lon != nil)
	if !hasIdentifier {
		return nil, &DatalasticError{Message: "one of name, uuid, unlocode, or lat/lon is required"}
	}
	addString(v, "name", p.Name)
	addString(v, "uuid", p.UUID)
	addString(v, "unlocode", p.Unlocode)
	addOptionalFloat(v, "lat", p.Lat)
	addOptionalFloat(v, "lon", p.Lon)
	addOptionalFloat(v, "radius", p.Radius)

	raw, err := r.client.do(r.client.baseV0, "port", v)
	if err != nil {
		return nil, err
	}
	return decodeInto[PortDetail](raw, "port")
}
