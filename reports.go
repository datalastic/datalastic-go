package datalastic

import (
	"net/url"
)

// ReportsResource manages asynchronous report jobs.
type ReportsResource struct {
	client *Client
}

// ReportSubmitParams describes a report job to submit.
type ReportSubmitParams struct {
	ReportType string
	Extra      map[string]interface{}
}

// Submit creates a new report job. The api-key is sent in the POST body.
func (r *ReportsResource) Submit(p ReportSubmitParams) (*Report, error) {
	if p.ReportType == "" {
		return nil, &DatalasticError{Message: "report_type is required"}
	}
	body := map[string]interface{}{
		"report_type": p.ReportType,
	}
	for k, val := range p.Extra {
		body[k] = val
	}
	raw, err := r.client.post(r.client.baseV0, "report", body)
	if err != nil {
		return nil, err
	}
	return decodeInto[Report](raw, "report")
}

// Get returns the status and result of a single report job.
func (r *ReportsResource) Get(reportID string) (*Report, error) {
	if reportID == "" {
		return nil, &DatalasticError{Message: "report_id is required"}
	}
	v := url.Values{}
	v.Set("report_id", reportID)
	raw, err := r.client.do(r.client.baseV0, "report", v)
	if err != nil {
		return nil, err
	}
	return decodeInto[Report](raw, "report")
}

// InRadiusHistoryParams describes an async historical area-scan report.
// Either Lat+Lon or PortUUID or PortUnlocode must be provided as the center.
type InRadiusHistoryParams struct {
	Lat          *float64
	Lon          *float64
	PortUUID     string
	PortUnlocode string
	Radius       float64 // required, kilometers
	From         string  // required, "YYYY-MM-DD"
	To           string  // required, "YYYY-MM-DD"
}

// InRadiusHistory submits an async report of all vessels that passed through a
// geographic area during the given time window. Poll with Reports.Get until
// status is done, then download the ZIP from the result URL.
func (r *ReportsResource) InRadiusHistory(p InRadiusHistoryParams) (*Report, error) {
	hasCenter := (p.Lat != nil && p.Lon != nil) || p.PortUUID != "" || p.PortUnlocode != ""
	if !hasCenter {
		return nil, &DatalasticError{Message: "a center is required: provide lat/lon, port_uuid, or port_unlocode"}
	}
	if p.Radius <= 0 {
		return nil, &DatalasticError{Message: "radius is required and must be greater than zero"}
	}
	if p.From == "" || p.To == "" {
		return nil, &DatalasticError{Message: "from and to dates are required"}
	}
	body := map[string]interface{}{
		"report_type": "inradius_history",
		"radius":      p.Radius,
		"from":        p.From,
		"to":          p.To,
	}
	if p.Lat != nil {
		body["lat"] = *p.Lat
	}
	if p.Lon != nil {
		body["lon"] = *p.Lon
	}
	if p.PortUUID != "" {
		body["port_uuid"] = p.PortUUID
	}
	if p.PortUnlocode != "" {
		body["port_unlocode"] = p.PortUnlocode
	}
	raw, err := r.client.post(r.client.baseV0, "report", body)
	if err != nil {
		return nil, err
	}
	return decodeInto[Report](raw, "report")
}

// ListAll returns all report jobs for the API key.
func (r *ReportsResource) ListAll() ([]Report, error) {
	v := url.Values{}
	v.Set("report_id", "_all")
	raw, err := r.client.do(r.client.baseV0, "report", v)
	if err != nil {
		return nil, err
	}
	return decodeSlice[Report](raw, "report")
}
