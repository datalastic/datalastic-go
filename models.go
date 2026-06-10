package datalastic

// Vessel represents basic AIS tracking data.
type Vessel struct {
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
	Total   int      `json:"total"`
	Vessels []Vessel `json:"vessels"`
}

// VesselInRadiusResult is returned by the in-radius scan endpoint.
type VesselInRadiusResult struct {
	Point struct {
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Radius float64 `json:"radius"`
	} `json:"point"`
	Total   int              `json:"total"`
	Vessels []VesselWithDist `json:"vessels"`
	// Next is the pagination token from meta.next; populated from the response
	// envelope, not the data body, so it carries no json tag.
	Next string
}

// VesselWithDist is a Vessel with an additional distance field.
type VesselWithDist struct {
	Vessel
	Distance float64 `json:"distance"`
}

// VesselHistory contains historical position data for a vessel.
type VesselHistory struct {
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
// matched vessels; Next is the meta.next pagination token for the next page.
type VesselFindResult struct {
	Items []VesselInfo
	Next  string
}

// VesselInfo contains static vessel specifications.
type VesselInfo struct {
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
	From  SeaRoutePoint      `json:"from"`
	Route SeaRouteLineString `json:"route"`
	To    SeaRoutePoint      `json:"to"`
}

// ApiStat contains API key usage statistics.
type ApiStat struct {
	UserID            string `json:"user_id"`
	KeyStatus         string `json:"key_status"`
	RequestsMade      int    `json:"requests_made"`
	RequestsRemaining int    `json:"requests_remaining"`
}

// Report represents an async report job.
type Report struct {
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
