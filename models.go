package datalastic

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Meta is the metadata half of the API response envelope. Typed fields cover
// the documented keys; Raw holds every key of the meta object verbatim, so
// counters and flags that this SDK version does not model yet stay reachable
// without waiting for a release.
//
// A response that carries no meta object yields a non-nil Meta with Success nil
// and an empty Raw.
type Meta struct {
	// Success is nil when the response carried no explicit success key.
	Success *bool
	// Message is the API-supplied message, typically set when Success is false.
	Message string
	// Next is the pagination token for the next page, empty on the last page.
	Next string
	// Endpoint is the endpoint path the API reports having served.
	Endpoint string
	// Duration is the server-side processing time in seconds. It tolerates a
	// JSON number or a numeric string; an unparseable value leaves it zero.
	Duration float64
	// Raw is every key of the meta object, unparsed.
	Raw map[string]json.RawMessage
}

// UnmarshalJSON populates Raw and the typed fields together, so the two views
// of the metadata can never disagree. Individual keys that cannot be parsed
// into their typed field are left at their zero value and remain available in
// Raw; a meta value that is not a JSON object is an error.
func (m *Meta) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("meta is not a JSON object: %w", err)
	}
	*m = Meta{Raw: raw}
	if v, ok := raw["success"]; ok {
		if parsed, ok := jsonBool(v); ok {
			m.Success = &parsed
		}
	}
	m.Message = jsonString(raw["message"])
	m.Next = jsonString(raw["next"])
	m.Endpoint = jsonString(raw["endpoint"])
	if v, ok := raw["duration"]; ok {
		if parsed, ok := jsonFloat(v); ok {
			m.Duration = parsed
		}
	}
	return nil
}

// jsonString decodes a JSON string, returning "" for absent or non-string
// values.
func jsonString(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// jsonBool decodes a JSON boolean, also accepting a quoted boolean.
func jsonBool(v json.RawMessage) (bool, bool) {
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		return b, true
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		if parsed, err := strconv.ParseBool(s); err == nil {
			return parsed, true
		}
	}
	return false, false
}

// jsonFloat decodes a JSON number, also accepting a quoted number.
func jsonFloat(v json.RawMessage) (float64, bool) {
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// Vessel represents basic AIS tracking data.
type Vessel struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	//
	// VesselPro and VesselEstimated embed Vessel, so they expose this field by
	// promotion rather than declaring their own.
	Meta *Meta `json:"-"`

	UUID              string  `json:"uuid"`
	Name              string  `json:"name"`
	MMSI              string  `json:"mmsi"`
	IMO               string  `json:"imo"`
	ENI               *string `json:"eni"`
	CountryISO        string  `json:"country_iso"`
	Type              string  `json:"type"`
	TypeSpecific      string  `json:"type_specific"`
	Lat               float64 `json:"lat"`
	Lon               float64 `json:"lon"`
	Speed             float64 `json:"speed"`
	Course            float64 `json:"course"`
	NavigationStatus  *string `json:"navigation_status"`
	Heading           float64 `json:"heading"`
	Destination       string  `json:"destination"`
	LastPositionEpoch int64   `json:"last_position_epoch"`
	LastPositionUTC   string  `json:"last_position_UTC"`
}

// VesselPro extends Vessel with ETA, ATD, port info, and draught.
type VesselPro struct {
	Vessel
	CurrentDraught   float64 `json:"current_draught"`
	DestPort         string  `json:"dest_port"`
	DestPortUnlocode string  `json:"dest_port_unlocode"`
	DepPort          string  `json:"dep_port"`
	DepPortUnlocode  string  `json:"dep_port_unlocode"`
	ATDEpoch         int64   `json:"atd_epoch"`
	ATDUTC           string  `json:"atd_UTC"`
	ETAEpoch         int64   `json:"eta_epoch"`
	ETAUTC           string  `json:"eta_UTC"`
}

// VesselEstimated extends VesselPro with a satellite-estimated position.
type VesselEstimated struct {
	VesselPro
	EstimatedPosition struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"estimated_position"`
}

// VesselBulkResult is returned by the bulk tracking endpoint.
type VesselBulkResult struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	Total   int      `json:"total"`
	Vessels []Vessel `json:"vessels"`
}

// VesselInRadiusResult is returned by the in-radius scan endpoint.
type VesselInRadiusResult struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	Point struct {
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Radius float64 `json:"radius"`
	} `json:"point"`
	Total   int              `json:"total"`
	Vessels []VesselWithDist `json:"vessels"`
	// Next mirrors Meta.Next, the pagination token for the next page. It comes
	// from the response envelope, not the data body, so it carries no json tag.
	Next string
}

// VesselWithDist is a Vessel with an additional distance field.
//
// It appears only inside the Vessels slice of an in-radius result, so its
// promoted Meta field stays nil: the envelope metadata belongs to the enclosing
// VesselInRadiusResult, which carries it.
type VesselWithDist struct {
	Vessel
	Distance float64 `json:"distance"`
}

// VesselHistory contains historical position data for a vessel.
type VesselHistory struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	UUID         string           `json:"uuid"`
	Name         string           `json:"name"`
	MMSI         string           `json:"mmsi"`
	IMO          string           `json:"imo"`
	ENI          *string          `json:"eni"`
	CountryISO   string           `json:"country_iso"`
	Type         string           `json:"type"`
	TypeSpecific string           `json:"type_specific"`
	Positions    []VesselPosition `json:"positions"`
}

// VesselPosition is a single historical position record.
type VesselPosition struct {
	Lat               float64 `json:"lat"`
	Lon               float64 `json:"lon"`
	Speed             float64 `json:"speed"`
	Course            float64 `json:"course"`
	Heading           float64 `json:"heading"`
	Destination       string  `json:"destination"`
	LastPositionEpoch int64   `json:"last_position_epoch"`
	LastPositionUTC   string  `json:"last_position_UTC"`
}

// VesselFindResult is returned by the vessel_find endpoint. Items holds the
// matched vessels; Next mirrors Meta.Next, the pagination token for the next
// page.
type VesselFindResult struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	Items []VesselInfo
	Next  string
}

// VesselInfo contains static vessel specifications.
type VesselInfo struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	NameAIS      string   `json:"name_ais"`
	MMSI         string   `json:"mmsi"`
	IMO          string   `json:"imo"`
	ENI          *string  `json:"eni"`
	CountryISO   string   `json:"country_iso"`
	CountryName  string   `json:"country_name"`
	Callsign     string   `json:"callsign"`
	Type         string   `json:"type"`
	TypeSpecific string   `json:"type_specific"`
	GrossTonnage *int     `json:"gross_tonnage"`
	Deadweight   *int     `json:"deadweight"`
	TEU          *string  `json:"teu"`
	LiquidGas    *float64 `json:"liquid_gas"`
	Length       *float64 `json:"length"`
	Breadth      *float64 `json:"breadth"`
	DraughtAvg   *float64 `json:"draught_avg"`
	DraughtMax   *float64 `json:"draught_max"`
	SpeedAvg     *float64 `json:"speed_avg"`
	SpeedMax     *float64 `json:"speed_max"`
	YearBuilt    string   `json:"year_built"`
	IsNavaid     bool     `json:"is_navaid"`
	HomePort     string   `json:"home_port"`
}

// Port represents a maritime port from the port_find endpoint.
type Port struct {
	UUID        string  `json:"uuid"`
	PortName    string  `json:"port_name"`
	CountryISO  string  `json:"country_iso"`
	CountryName string  `json:"country_name"`
	Unlocode    string  `json:"unlocode"`
	PortType    string  `json:"port_type"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	AreaLvl1    string  `json:"area_lvl1"`
	AreaLvl2    string  `json:"area_lvl2"`
}

// Terminal represents a container terminal within a port.
type Terminal struct {
	TerminalCode string  `json:"terminal_code"`
	TerminalName string  `json:"terminal_name"`
	CompanyName  string  `json:"company_name"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	URL          string  `json:"url"`
	Address      string  `json:"address"`
}

// PortDetail is the response from the /port endpoint, including terminals.
type PortDetail struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	Port
	Terminals []Terminal `json:"terminals"`
}

// SeaRoutePoint is a GeoJSON Feature for origin/destination.
type SeaRoutePoint struct {
	Type       string                 `json:"type"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// SeaRouteLineString is a GeoJSON Feature for the route.
type SeaRouteLineString struct {
	Type       string                 `json:"type"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties struct {
		TotalDist float64 `json:"total_dist"`
	} `json:"properties"`
}

// SeaRoute is the result of a sea route calculation.
type SeaRoute struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	From  SeaRoutePoint      `json:"from"`
	Route SeaRouteLineString `json:"route"`
	To    SeaRoutePoint      `json:"to"`
}

// ApiStat contains API key usage statistics.
type ApiStat struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	UserID            string `json:"user_id"`
	KeyStatus         string `json:"key_status"`
	RequestsMade      int    `json:"requests_made"`
	RequestsRemaining int    `json:"requests_remaining"`
}

// Report represents an async report job.
type Report struct {
	// Meta is the response envelope metadata for the call that produced this
	// value. It is populated from the envelope, not the data body.
	Meta *Meta `json:"-"`

	ReportID   string                 `json:"report_id"`
	ReportType string                 `json:"report_type"`
	Status     string                 `json:"status"`
	ResultURL  string                 `json:"result_url,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at,omitempty"`
	Params     map[string]interface{} `json:"params,omitempty"`
}

// DryDockRecord from maritime_reports/dry_dock_dates
type DryDockRecord struct {
	ID                int     `json:"id"`
	IMO               string  `json:"imo"`
	VesselName        string  `json:"vessel_name"`
	SpecialSurveyDate string  `json:"special_survey_date"`
	DryDockDate       string  `json:"dry_dock_date"`
	IOPPIssueDate     string  `json:"iopp_issue_date"`
	IOPPExpDate       string  `json:"iopp_exp_date"`
	TechnicalManager  string  `json:"technical_manager"`
	CountryCode       string  `json:"country_code"`
	Website           string  `json:"website"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	Address           string  `json:"address"`
	LinkedIn          *string `json:"linkedin"`
	ModifiedAt        string  `json:"modified_at"`
}

// CasualtyRecord from maritime_reports/casualty
type CasualtyRecord struct {
	ID              int    `json:"id"`
	IMO             string `json:"imo"`
	VesselName      string `json:"vessel_name"`
	CasualtyDate    string `json:"casualty_date"`
	CasualtyType    string `json:"casualty_type"`
	CasualtyDetails string `json:"casualty_details"`
	ModifiedAt      string `json:"modified_at"`
}

// InspectionRecord from maritime_reports/inspections
type InspectionRecord struct {
	ID                    int    `json:"id"`
	IMO                   string `json:"imo"`
	VesselName            string `json:"vessel_name"`
	VesselTypeCode        string `json:"vessel_type_code"`
	FlagCode              string `json:"flag_code"`
	InspectionDate        string `json:"inspection_date"`
	InspectionAuthority   string `json:"inspection_authority"`
	InspectionPort        string `json:"inspection_port"`
	InspectionType        string `json:"inspection_type"`
	Detention             string `json:"detention"`
	ShipDeficiencies      string `json:"ship_deficiencies"`
	DeficiencyDescription string `json:"deficiency_description"`
	TechnicalISMManager   string `json:"technical_ism_manager"`
	CountryCode           string `json:"country_code"`
	Website               string `json:"website"`
	Email                 string `json:"email"`
	Phone                 string `json:"phone"`
	Address               string `json:"address"`
	CompanyIMO            string `json:"company_imo"`
	ModifiedAt            string `json:"modified_at"`
}

// SPDRecord from maritime_reports/spd (sales, purchases, demolitions)
type SPDRecord struct {
	ID                  int      `json:"id"`
	SalesReportDate     string   `json:"sales_report_date"`
	IMO                 string   `json:"imo"`
	VesselName          string   `json:"vessel_name"`
	FlagName            string   `json:"flag_name"`
	VesselTypeCode      string   `json:"vessel_type_code"`
	BuiltYear           string   `json:"built_year"`
	DWTDesign           int      `json:"dwt_design"`
	GT                  int      `json:"gt"`
	LDT                 int      `json:"ldt"`
	Seller              string   `json:"seller"`
	Buyer               string   `json:"buyer"`
	SalesPriceUSDMio    *float64 `json:"sales_price_usd_mio"`
	SalesPriceUSDPerLDT float64  `json:"sales_price_usd/ldt"`
	Destination         string   `json:"destination"`
	SalesType           string   `json:"sales_type"`
	DryDockDate         string   `json:"dry_dock_date"`
	SpecialSurveyDate   string   `json:"special_survey_date"`
	SalesNote           *string  `json:"sales_note"`
	PreviousSalesRecord *string  `json:"previous_sales_record"`
	ModifiedAt          string   `json:"modified_at"`
}

// OwnershipRecord from maritime_reports/ownership
type OwnershipRecord struct {
	ID                       int     `json:"id"`
	IMO                      string  `json:"imo"`
	VesselName               string  `json:"vessel_name"`
	BeneficialOwner          string  `json:"beneficial_owner"`
	BeneficialOwnerCountry   string  `json:"beneficial_owner_country"`
	Operator                 string  `json:"operator"`
	OperatorCountry          string  `json:"operator_country"`
	FlagName                 *string `json:"flag_name"`
	VesselTypeCode           string  `json:"vessel_type_code"`
	BuiltYear                string  `json:"built_year"`
	Buyer                    *string `json:"buyer"`
	DWTDesign                int     `json:"dwt_design"`
	Class1Code               *string `json:"class1_code"`
	TechnicalManager         *string `json:"technical_manager"`
	TechnicalManagerCountry  *string `json:"technical_manager_country"`
	CommercialManager        string  `json:"commercial_manager"`
	CommercialManagerCountry string  `json:"commercial_manager_country"`
	ModifiedAt               string  `json:"modified_at"`
}

// ClassSocietyRecord from maritime_reports/class_society
type ClassSocietyRecord struct {
	IMO                 string  `json:"imo"`
	VesselName          string  `json:"vessel_name"`
	VesselTypeCode      string  `json:"vessel_type_code"`
	FlagName            string  `json:"flag_name"`
	BuiltYear           string  `json:"built_year"`
	DWTDesign           int     `json:"dwt_design"`
	SpecialSurveyDate   *string `json:"special_survey_date"`
	DryDockDate         *string `json:"dry_dock_date"`
	Class1Code          string  `json:"class1_code"`
	BeneficialOwnerIMO  string  `json:"beneficial_owner_imo"`
	BeneficialOwner     string  `json:"beneficial_owner"`
	TechnicalManagerIMO string  `json:"technical_manager_imo"`
	TechnicalManager    string  `json:"technical_manager"`
	DraftDesign         float64 `json:"draft_design"`
	NT                  int     `json:"nt"`
	GT                  int     `json:"gt"`
	LOA                 float64 `json:"loa"`
	LBP                 float64 `json:"lbp"`
	Depth               float64 `json:"depth"`
	BeamModuled         float64 `json:"beam_moduled"`
	EngineBuilder       string  `json:"engine_builder"`
	EngineDesigner      string  `json:"engine_designer"`
	PropulsionTypeCode  string  `json:"propulsion_type_code"`
	ModifiedAt          string  `json:"modified_at"`
}

// EngineRecord from maritime_reports/engine
type EngineRecord struct {
	IMO                 string `json:"imo"`
	VesselName          string `json:"vessel_name"`
	VesselTypeCode      string `json:"vessel_type_code"`
	PropulsionTypeCode  string `json:"propulsion_type_code"`
	MCO                 int    `json:"mco"`
	MCOUnit             string `json:"mco_unit"`
	MCORPM              int    `json:"mco_rpm"`
	TradingCategoryCode string `json:"trading_category_code"`
	BuiltYear           string `json:"built_year"`
	GT                  int    `json:"gt"`
	EngineDesignation   string `json:"engine_designation"`
	EngineBuilder       string `json:"engine_builder"`
	EngineDesigner      string `json:"engine_designer"`
	ModifiedAt          string `json:"modified_at"`
}

// CompanyRecord from maritime_reports/companies
type CompanyRecord struct {
	ID                int     `json:"id"`
	ShortName         string  `json:"short_name"`
	LongName          string  `json:"long_name"`
	CompanyType       string  `json:"company_type"`
	CountryCode       string  `json:"country_code"`
	CompanyIMO        string  `json:"company_imo"`
	Website           string  `json:"website"`
	CompanyStatus     string  `json:"company_status"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	Address           string  `json:"address"`
	LinkedIn          *string `json:"linkedin"`
	ParentCompanyIMO  *string `json:"parent_company_imo"`
	ParentCompanyName *string `json:"parent_company_name"`
	ModifiedAt        string  `json:"modified_at"`
}
