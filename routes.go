package datalastic

import (
	"net/url"
)

// RoutesResource groups sea route calculation endpoints.
type RoutesResource struct {
	client *Client
}

// RouteParams describes an origin and destination for a sea route. Each endpoint
// may be a coordinate or a port (by UUID or UN/LOCODE).
type RouteParams struct {
	LatFrom          *float64
	LonFrom          *float64
	PortUUIDFrom     string
	PortUnlocodeFrom string
	LatTo            *float64
	LonTo            *float64
	PortUUIDTo       string
	PortUnlocodeTo   string
}

// Calculate computes a sea route between two points. Uses the extended base URL.
func (r *RoutesResource) Calculate(p RouteParams) (*SeaRoute, error) {
	hasFrom := (p.LatFrom != nil && p.LonFrom != nil) || p.PortUUIDFrom != "" || p.PortUnlocodeFrom != ""
	hasTo := (p.LatTo != nil && p.LonTo != nil) || p.PortUUIDTo != "" || p.PortUnlocodeTo != ""
	if !hasFrom {
		return nil, &DatalasticError{Message: "an origin is required: provide lat_from/lon_from or a port"}
	}
	if !hasTo {
		return nil, &DatalasticError{Message: "a destination is required: provide lat_to/lon_to or a port"}
	}

	v := url.Values{}
	addOptionalFloat(v, "lat_from", p.LatFrom)
	addOptionalFloat(v, "lon_from", p.LonFrom)
	addString(v, "port_uuid_from", p.PortUUIDFrom)
	addString(v, "port_unlocode_from", p.PortUnlocodeFrom)
	addOptionalFloat(v, "lat_to", p.LatTo)
	addOptionalFloat(v, "lon_to", p.LonTo)
	addString(v, "port_uuid_to", p.PortUUIDTo)
	addString(v, "port_unlocode_to", p.PortUnlocodeTo)

	raw, err := r.client.do(r.client.baseExt, "route", v)
	if err != nil {
		return nil, err
	}
	return decodeInto[SeaRoute](raw, "route")
}
