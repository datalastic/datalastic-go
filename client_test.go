package datalastic

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// capture records the request the test server received.
type capture struct {
	path   string
	query  url.Values
	body   map[string]interface{}
	method string
}

// newServer returns an httptest.Server that records the request into cap and
// responds with the given status and body.
func newServer(t *testing.T, status int, body string, cap *capture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cap != nil {
			cap.path = r.URL.Path
			cap.query = r.URL.Query()
			cap.method = r.Method
			if r.Method == http.MethodPost {
				raw, _ := io.ReadAll(r.Body)
				m := map[string]interface{}{}
				_ = json.Unmarshal(raw, &m)
				cap.body = m
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dataEnvelope wraps a JSON value in the standard {"data": ..., "meta": ...}
// response envelope.
func dataEnvelope(inner string) string {
	return `{"data":` + inner + `,"meta":{"duration":0.1,"endpoint":"/test","success":true}}`
}

// mustClient builds a Client pointed at baseURL.
func mustClient(t *testing.T, baseURL string, opts ...Option) *Client {
	t.Helper()
	all := append([]Option{withBaseURLs(baseURL)}, opts...)
	c, err := NewClient("test-key", all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }

// dataEnvelopeWithNext wraps a JSON value in the standard envelope and includes
// a meta.next pagination token.
func dataEnvelopeWithNext(inner, next string) string {
	nextJSON, _ := json.Marshal(next) // produces a quoted, escaped JSON string
	return `{"data":` + inner + `,"meta":{"duration":0.1,"endpoint":"/test","success":true,"next":` + string(nextJSON) + `}}`
}

// --- NewClient ---

func TestNewClient_ValidKey(t *testing.T) {
	c, err := NewClient("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Vessels == nil || c.Ports == nil || c.Routes == nil || c.Intel == nil || c.Reports == nil {
		t.Fatal("resources not initialized")
	}
}

func TestNewClient_EmptyKey(t *testing.T) {
	_, err := NewClient("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	var de *DatalasticError
	if !errors.As(err, &de) {
		t.Fatalf("expected DatalasticError, got %T", err)
	}
}

func TestNewClient_WhitespaceKey(t *testing.T) {
	if _, err := NewClient("   "); err == nil {
		t.Fatal("expected error for whitespace key")
	}
}

// --- Stat & error mapping ---

func TestStat_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"user_id":"u1","key_status":"active","requests_made":10,"requests_remaining":90}`), cap)
	c := mustClient(t, srv.URL)

	stat, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.UserID != "u1" || stat.RequestsRemaining != 90 {
		t.Fatalf("unexpected stat: %+v", stat)
	}
	if cap.query.Get("api-key") != "test-key" {
		t.Fatalf("api-key not in query: %v", cap.query)
	}
	if cap.path != "/stat" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

func TestStat_ErrorCodes(t *testing.T) {
	cases := []struct {
		status int
		check  func(err error) bool
	}{
		{401, func(err error) bool { var e *AuthenticationError; return errors.As(err, &e) && e.StatusCode == 401 }},
		{402, func(err error) bool {
			var e *InsufficientCreditsError
			return errors.As(err, &e) && e.StatusCode == 402
		}},
		{404, func(err error) bool { var e *NotFoundError; return errors.As(err, &e) && e.StatusCode == 404 }},
		{429, func(err error) bool { var e *RateLimitError; return errors.As(err, &e) && e.StatusCode == 429 }},
		{500, func(err error) bool { var e *APIError; return errors.As(err, &e) && e.StatusCode == 500 }},
		{400, func(err error) bool { var e *APIError; return errors.As(err, &e) && e.StatusCode == 400 }},
	}
	for _, tc := range cases {
		srv := newServer(t, tc.status, `{"data":{"message":"boom"}}`, nil)
		c := mustClient(t, srv.URL)
		_, err := c.Stat()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !tc.check(err) {
			t.Fatalf("status %d: wrong error type %T: %v", tc.status, err, err)
		}
		// All typed errors must unwrap to DatalasticError.
		var de *DatalasticError
		if !errors.As(err, &de) {
			t.Fatalf("status %d: does not unwrap to DatalasticError", tc.status)
		}
	}
}

// --- Vessels.Get ---

func TestVessels_Get_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1","name":"TESTER","mmsi":"123","lat":1.5,"lon":2.5,"speed":10.2}`), cap)
	c := mustClient(t, srv.URL)

	v, err := c.Vessels.Get(VesselParams{MMSI: "123"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.UUID != "v-1" || v.Name != "TESTER" || v.Lat != 1.5 {
		t.Fatalf("unexpected vessel: %+v", v)
	}
	if cap.path != "/vessel" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
	if cap.query.Get("mmsi") != "123" {
		t.Fatalf("mmsi missing: %v", cap.query)
	}
}

func TestVessels_Get_MissingIdentifier(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Get(VesselParams{})
	if err == nil {
		t.Fatal("expected error for missing identifier")
	}
	var de *DatalasticError
	if !errors.As(err, &de) {
		t.Fatalf("expected DatalasticError, got %T", err)
	}
}

// --- Vessels.Pro ---

func TestVessels_Pro_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1","name":"PRO","mmsi":"123","current_draught":8.1,"dest_port":"ROTTERDAM","eta_epoch":1700000000}`), cap)
	c := mustClient(t, srv.URL)

	v, err := c.Vessels.Pro(VesselParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("Pro: %v", err)
	}
	if v.CurrentDraught != 8.1 || v.DestPort != "ROTTERDAM" || v.ETAEpoch != 1700000000 {
		t.Fatalf("unexpected pro: %+v", v)
	}
	if cap.path != "/vessel_pro" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

// --- Vessels.Bulk ---

func TestVessels_Bulk_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"total":2,"vessels":[{"uuid":"a"},{"uuid":"b"}]}`), cap)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.Bulk(VesselBulkParams{MMSI: []string{"111", "222"}})
	if err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	if res.Total != 2 || len(res.Vessels) != 2 {
		t.Fatalf("unexpected bulk: %+v", res)
	}
	mmsis := cap.query["mmsi"]
	if len(mmsis) != 2 || mmsis[0] != "111" || mmsis[1] != "222" {
		t.Fatalf("repeated mmsi params not sent: %v", cap.query)
	}
}

func TestVessels_Bulk_Empty(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Bulk(VesselBulkParams{})
	if err == nil {
		t.Fatal("expected error for empty bulk params")
	}
	var de *DatalasticError
	if !errors.As(err, &de) {
		t.Fatalf("expected DatalasticError, got %T", err)
	}
}

// --- Vessels.InRadius ---

func TestVessels_InRadius_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"point":{"lat":1,"lon":2,"radius":50},"total":1,"vessels":[{"uuid":"a","distance":12.3}]}`), cap)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 50})
	if err != nil {
		t.Fatalf("InRadius: %v", err)
	}
	if res.Total != 1 || res.Vessels[0].Distance != 12.3 {
		t.Fatalf("unexpected inradius: %+v", res)
	}
	if cap.query.Get("radius") != "50" {
		t.Fatalf("radius missing: %v", cap.query)
	}
}

func TestVessels_InRadius_MissingRadius(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2)})
	if err == nil {
		t.Fatal("expected error for missing radius")
	}
}

func TestVessels_InRadius_MissingCenter(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.InRadius(VesselInRadiusParams{Radius: 10})
	if err == nil {
		t.Fatal("expected error for missing center")
	}
}

func TestVessels_InRadius_IncludeNullType(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"point":{"lat":1,"lon":2,"radius":50},"total":0,"vessels":[]}`), cap)
	c := mustClient(t, srv.URL)

	_, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 50, IncludeNullType: boolPtr(false)})
	if err != nil {
		t.Fatalf("InRadius: %v", err)
	}
	if cap.query.Get("_empty_") != "false" {
		t.Fatalf("_empty_ not sent as false: %v", cap.query)
	}
}

func TestVessels_InRadius_PaginationNext(t *testing.T) {
	srv := newServer(t, 200, dataEnvelopeWithNext(`{"point":{"lat":1,"lon":2,"radius":50},"total":1,"vessels":[{"uuid":"a","distance":1}]}`, "ir-token"), nil)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 50})
	if err != nil {
		t.Fatalf("InRadius: %v", err)
	}
	if res.Next != "ir-token" {
		t.Fatalf("expected next token from meta, got %q", res.Next)
	}
}

// --- Vessels.History ---

func TestVessels_History_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1","name":"H","positions":[{"lat":1,"lon":2,"speed":5}]}`), cap)
	c := mustClient(t, srv.URL)

	h, err := c.Vessels.History(VesselHistoryParams{UUID: "v-1", Days: intPtr(7)})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(h.Positions) != 1 || h.Positions[0].Speed != 5 {
		t.Fatalf("unexpected history: %+v", h)
	}
	if cap.query.Get("days") != "7" {
		t.Fatalf("days missing: %v", cap.query)
	}
}

func TestVessels_History_MissingIdentifier(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.History(VesselHistoryParams{Days: intPtr(7)})
	if err == nil {
		t.Fatal("expected error for missing identifier")
	}
}

// --- Vessels.Info ---

func TestVessels_Info_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1","name":"INFO","gross_tonnage":50000,"teu":"8000","length":300.5}`), cap)
	c := mustClient(t, srv.URL)

	info, err := c.Vessels.Info(VesselParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.GrossTonnage == nil || *info.GrossTonnage != 50000 {
		t.Fatalf("gross tonnage wrong: %+v", info)
	}
	if info.TEU == nil || *info.TEU != "8000" {
		t.Fatalf("teu wrong: %+v", info)
	}
	if info.Length == nil || *info.Length != 300.5 {
		t.Fatalf("length wrong: %+v", info)
	}
	if cap.path != "/vessel_info" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

// --- Vessels.Find ---

func TestVessels_Find_HappyPath_TypeMapsToType(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"uuid":"v-1","name":"FIND"}]`), cap)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo", Fuzzy: intPtr(1)})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "FIND" {
		t.Fatalf("unexpected find: %+v", res)
	}
	if cap.query.Get("type") != "cargo" {
		t.Fatalf("VesselType did not map to 'type': %v", cap.query)
	}
	if cap.query.Get("fuzzy") != "1" {
		t.Fatalf("fuzzy not passed: %v", cap.query)
	}
}

func TestVessels_Find_IncludeNullType(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"uuid":"v-1","name":"FIND"}]`), cap)
	c := mustClient(t, srv.URL)

	_, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo", IncludeNullType: boolPtr(true)})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cap.query.Get("_empty_") != "true" {
		t.Fatalf("_empty_ not sent as true: %v", cap.query)
	}
}

func TestVessels_Find_IncludeNullType_CountsAsSearch(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)

	// IncludeNullType alone, with no other filter, must satisfy the guard.
	_, err := c.Vessels.Find(VesselFindParams{IncludeNullType: boolPtr(false)})
	if err != nil {
		t.Fatalf("IncludeNullType should satisfy search guard: %v", err)
	}
	if cap.query.Get("_empty_") != "false" {
		t.Fatalf("_empty_ not sent as false: %v", cap.query)
	}
}

func TestVessels_Find_IncludeNullType_TrueSatisfiesGuard(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Find(VesselFindParams{IncludeNullType: boolPtr(true)})
	if err != nil {
		t.Fatalf("IncludeNullType=true alone should satisfy guard: %v", err)
	}
	if cap.query.Get("_empty_") != "true" {
		t.Fatalf("_empty_ not set to true: %v", cap.query)
	}
}

func TestVessels_Find_NextParamIsForwarded(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo", Next: "tok123"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cap.query.Get("next") != "tok123" {
		t.Fatalf("next param not forwarded: %v", cap.query)
	}
}

func TestVessels_InRadius_NextParamIsForwarded(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"point":{"lat":1,"lon":2,"radius":10},"total":0,"vessels":[]}`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 10, Next: "page2"})
	if err != nil {
		t.Fatalf("InRadius: %v", err)
	}
	if cap.query.Get("next") != "page2" {
		t.Fatalf("next param not forwarded: %v", cap.query)
	}
}

func TestVessels_Find_PaginationNext(t *testing.T) {
	srv := newServer(t, 200, dataEnvelopeWithNext(`[{"uuid":"v-1","name":"FIND"}]`, "page-token-2"), nil)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if res.Next != "page-token-2" {
		t.Fatalf("expected next token from meta, got %q", res.Next)
	}
}

func TestVessels_Find_RequiresSearchParam(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[]`), nil)
	c := mustClient(t, srv.URL)
	// Fuzzy and Next alone must not satisfy the guard.
	_, err := c.Vessels.Find(VesselFindParams{Fuzzy: intPtr(1), Next: "abc"})
	if err == nil {
		t.Fatal("expected error when only fuzzy/next provided")
	}
}

func TestVessels_Find_RangeCountsAsSearch(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Find(VesselFindParams{GrossTonnageMin: intPtr(1000)})
	if err != nil {
		t.Fatalf("range bound should satisfy guard: %v", err)
	}
	if cap.query.Get("gross_tonnage_min") != "1000" {
		t.Fatalf("range param missing: %v", cap.query)
	}
}

// --- Vessels.Estimated (BaseExt) ---

func TestVessels_Estimated_UsesBaseExt(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1","estimated_position":{"lat":3.3,"lon":4.4}}`), cap)
	c := mustClient(t, srv.URL)

	v, err := c.Vessels.Estimated(VesselParams{UUID: "v-1"})
	if err != nil {
		t.Fatalf("Estimated: %v", err)
	}
	if v.EstimatedPosition.Lat != 3.3 {
		t.Fatalf("unexpected estimated: %+v", v)
	}
	if cap.path != "/vessel_pro_est" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

func TestVessels_Estimated_RoutesToExtBase(t *testing.T) {
	// Verify the SDK actually targets the ext base by giving v0 and ext
	// different servers.
	cap := &capture{}
	extSrv := newServer(t, 200, dataEnvelope(`{"uuid":"v-1"}`), cap)
	c, err := NewClient("test-key")
	if err != nil {
		t.Fatal(err)
	}
	c.baseV0 = "http://127.0.0.1:1" // unreachable
	c.baseExt = extSrv.URL
	c.baseMR = "http://127.0.0.1:1"
	if _, err := c.Vessels.Estimated(VesselParams{UUID: "v-1"}); err != nil {
		t.Fatalf("Estimated should hit ext base: %v", err)
	}
	if cap.path != "/vessel_pro_est" {
		t.Fatalf("ext server not hit: %s", cap.path)
	}
}

// --- Ports ---

func TestPorts_Find_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"uuid":"p-1","port_name":"ROTTERDAM","unlocode":"NLRTM"}]`), cap)
	c := mustClient(t, srv.URL)

	res, err := c.Ports.Find(PortFindParams{CountryISO: "NL"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(res) != 1 || res[0].PortName != "ROTTERDAM" {
		t.Fatalf("unexpected ports: %+v", res)
	}
	if cap.query.Get("country_iso") != "NL" {
		t.Fatalf("country_iso missing: %v", cap.query)
	}
}

func TestPorts_Find_RequiresSearchParam(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[]`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Ports.Find(PortFindParams{Fuzzy: intPtr(1)})
	if err == nil {
		t.Fatal("expected error when only fuzzy provided")
	}
}

func TestPorts_Get_HappyPath(t *testing.T) {
	cap := &capture{}
	inner := `{"uuid":"p-1","port_name":"ROTTERDAM","terminals":[{"terminal_code":"T1","terminal_name":"APM"}]}`
	srv := newServer(t, 200, dataEnvelope(inner), cap)
	c := mustClient(t, srv.URL)

	pd, err := c.Ports.Get(PortGetParams{Unlocode: "NLRTM"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pd.PortName != "ROTTERDAM" || len(pd.Terminals) != 1 || pd.Terminals[0].TerminalName != "APM" {
		t.Fatalf("unexpected port detail: %+v", pd)
	}
	if cap.path != "/port" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

func TestPorts_Get_MissingIdentifier(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	if _, err := c.Ports.Get(PortGetParams{}); err == nil {
		t.Fatal("expected error for missing identifier")
	}
}

// --- Routes (BaseExt) ---

func TestRoutes_Calculate_UsesBaseExt(t *testing.T) {
	cap := &capture{}
	inner := `{"from":{"type":"Feature"},"to":{"type":"Feature"},"route":{"type":"Feature","properties":{"total_dist":1234.5}}}`
	srv := newServer(t, 200, dataEnvelope(inner), cap)
	c := mustClient(t, srv.URL)

	r, err := c.Routes.Calculate(RouteParams{
		LatFrom: floatPtr(1), LonFrom: floatPtr(2),
		LatTo: floatPtr(3), LonTo: floatPtr(4),
	})
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if r.Route.Properties.TotalDist != 1234.5 {
		t.Fatalf("unexpected route: %+v", r)
	}
	if cap.path != "/route" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

func TestRoutes_Calculate_MissingEndpoint(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	if _, err := c.Routes.Calculate(RouteParams{LatFrom: floatPtr(1), LonFrom: floatPtr(2)}); err == nil {
		t.Fatal("expected error for missing destination")
	}
}

// --- Intel (BaseMR) ---

func TestIntel_DryDock_UsesBaseMR(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"imo":"9999","vessel_name":"DD"}]`), cap)
	c := mustClient(t, srv.URL)

	res, err := c.Intel.DryDock(IntelDryDockParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("DryDock: %v", err)
	}
	if len(res) != 1 || res[0].VesselName != "DD" {
		t.Fatalf("unexpected drydock: %+v", res)
	}
	if cap.path != "/dry_dock_dates" {
		t.Fatalf("unexpected path: %s", cap.path)
	}
}

func TestIntel_Casualties_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"imo":"9999","casualty_type":"FIRE"}]`), cap)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.Casualties(IntelDateRangeParams{IMO: "9999", From: "2024-01-01", To: "2024-12-31"})
	if err != nil {
		t.Fatalf("Casualties: %v", err)
	}
	if res[0].CasualtyType != "FIRE" {
		t.Fatalf("unexpected casualty: %+v", res)
	}
	if cap.query.Get("from") != "2024-01-01" || cap.query.Get("to") != "2024-12-31" {
		t.Fatalf("from/to not passed: %v", cap.query)
	}
}

func TestIntel_Inspections_HappyPath(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"imo":"9999","detention":"N"}]`), nil)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.Inspections(IntelDateRangeParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("Inspections: %v", err)
	}
	if res[0].Detention != "N" {
		t.Fatalf("unexpected inspection: %+v", res)
	}
}

func TestIntel_SPD_HappyPath(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"imo":"9999","sales_type":"SALE","sales_price_usd/ldt":450.5}]`), nil)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.SPD(IntelDateRangeParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("SPD: %v", err)
	}
	if res[0].SalesType != "SALE" || res[0].SalesPriceUSDPerLDT != 450.5 {
		t.Fatalf("unexpected spd: %+v", res)
	}
}

func TestIntel_Ownership_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"imo":"9999","beneficial_owner":"ACME"}]`), cap)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.Ownership(OwnershipParams{IMO: "9999", BeneficialOwner: "ACME"})
	if err != nil {
		t.Fatalf("Ownership: %v", err)
	}
	if res[0].BeneficialOwner != "ACME" {
		t.Fatalf("unexpected ownership: %+v", res)
	}
	if cap.query.Get("beneficial_owner") != "ACME" {
		t.Fatalf("beneficial_owner not passed: %v", cap.query)
	}
}

func TestIntel_ClassSociety_HappyPath(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[{"imo":"9999","class1_code":"DNV","gt":50000}]`), nil)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.ClassSociety(ClassSocietyParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("ClassSociety: %v", err)
	}
	if res[0].Class1Code != "DNV" || res[0].GT != 50000 {
		t.Fatalf("unexpected class society: %+v", res)
	}
}

func TestIntel_Engine_HappyPath(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`[{"imo":"9999","mco":12000,"mco_unit":"kW"}]`), nil)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.Engine(EngineParams{IMO: "9999"})
	if err != nil {
		t.Fatalf("Engine: %v", err)
	}
	if res[0].MCO != 12000 || res[0].MCOUnit != "kW" {
		t.Fatalf("unexpected engine: %+v", res)
	}
}

func TestIntel_Companies_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"id":1,"short_name":"ACME","company_imo":"1234567"}]`), cap)
	c := mustClient(t, srv.URL)
	res, err := c.Intel.Companies(CompanyParams{CompanyIMO: "1234567"})
	if err != nil {
		t.Fatalf("Companies: %v", err)
	}
	if res[0].ShortName != "ACME" {
		t.Fatalf("unexpected company: %+v", res)
	}
	if cap.query.Get("company_imo") != "1234567" {
		t.Fatalf("company_imo not passed: %v", cap.query)
	}
}

// --- Reports ---

func TestReports_Submit_PostsApiKeyInBody(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"report_id":"r-1","report_type":"fleet","status":"queued"}`), cap)
	c := mustClient(t, srv.URL)

	rep, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet", Extra: map[string]interface{}{"imo": "9999"}})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if rep.ReportID != "r-1" || rep.Status != "queued" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if cap.method != http.MethodPost {
		t.Fatalf("expected POST, got %s", cap.method)
	}
	if cap.body["api-key"] != "test-key" {
		t.Fatalf("api-key not in body: %v", cap.body)
	}
	if cap.body["report_type"] != "fleet" || cap.body["imo"] != "9999" {
		t.Fatalf("body fields missing: %v", cap.body)
	}
}

func TestReports_Submit_RequiresType(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	if _, err := c.Reports.Submit(ReportSubmitParams{}); err == nil {
		t.Fatal("expected error for missing report_type")
	}
}

func TestReports_Get_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"report_id":"r-1","status":"done","result_url":"http://x"}`), cap)
	c := mustClient(t, srv.URL)

	rep, err := c.Reports.Get("r-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rep.Status != "done" || rep.ResultURL != "http://x" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if cap.query.Get("report_id") != "r-1" {
		t.Fatalf("report_id missing: %v", cap.query)
	}
}

func TestReports_Get_RequiresID(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	if _, err := c.Reports.Get(""); err == nil {
		t.Fatal("expected error for empty report id")
	}
}

func TestReports_ListAll_UsesAllSentinel(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[{"report_id":"r-1"},{"report_id":"r-2"}]`), cap)
	c := mustClient(t, srv.URL)

	reps, err := c.Reports.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(reps) != 2 {
		t.Fatalf("unexpected reports: %+v", reps)
	}
	if cap.query.Get("report_id") != "_all" {
		t.Fatalf("expected report_id=_all, got %v", cap.query)
	}
}

// --- Options ---

func TestWithTimeout_ClonesClient(t *testing.T) {
	shared := &http.Client{Timeout: 99 * time.Second}
	c, err := NewClient("k", WithHTTPClient(shared), WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("timeout not applied: %v", c.httpClient.Timeout)
	}
	if shared.Timeout != 99*time.Second {
		t.Fatalf("shared client was mutated: %v", shared.Timeout)
	}
}

func TestWithTimeout_Enforced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, dataEnvelope(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := mustClient(t, srv.URL, WithTimeout(20*time.Millisecond))
	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError on timeout, got %T", err)
	}
}

// --- Transport / parsing errors ---

func TestConnectionError_IsAPIError(t *testing.T) {
	c, err := NewClient("k", withBaseURLs("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Stat()
	if err == nil {
		t.Fatal("expected connection error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
}

func TestNonJSONResponse_IsAPIError(t *testing.T) {
	srv := newServer(t, 200, `this is not json`, nil)
	c := mustClient(t, srv.URL)
	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
}

func TestMissingDataKey_IsAPIError(t *testing.T) {
	srv := newServer(t, 200, `{"meta":{"success":true}}`, nil)
	c := mustClient(t, srv.URL)
	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected error for missing data key")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
}

func TestNullDataKey_IsAPIError(t *testing.T) {
	srv := newServer(t, 200, `{"data":null,"meta":{"success":true}}`, nil)
	c := mustClient(t, srv.URL)
	if _, err := c.Stat(); err == nil {
		t.Fatal("expected error for null data")
	}
}

// --- Error type plumbing ---

func TestDatalasticError_Message(t *testing.T) {
	e := &DatalasticError{Message: "boom"}
	if e.Error() != "boom" {
		t.Fatalf("unexpected message: %q", e.Error())
	}
}

func TestErrorMessage_Extraction(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"data":{"message":"data message"}}`, "data message"},
		{`{"data":{"detail":"data detail"}}`, "data detail"},
		{`{"message":"top message"}`, "top message"},
		{`{"error":"top error"}`, "top error"},
		{`not json at all`, "bad request"}, // falls back to status text for 400
	}
	for _, tc := range cases {
		srv := newServer(t, 400, tc.body, nil)
		c := mustClient(t, srv.URL)
		_, err := c.Stat()
		if err == nil {
			t.Fatalf("body %q: expected error", tc.body)
		}
		var de *DatalasticError
		if !errors.As(err, &de) || de.Message != tc.want {
			t.Fatalf("body %q: got message %q, want %q", tc.body, de.Message, tc.want)
		}
	}
}

// --- Decode failure ---

func TestDecodeFailure_IsAPIError(t *testing.T) {
	// data is a JSON array where the SDK expects a Vessel object.
	srv := newServer(t, 200, dataEnvelope(`[1,2,3]`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Vessels.Get(VesselParams{IMO: "9999"})
	if err == nil {
		t.Fatal("expected decode error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
}

// --- Full param coverage ---

func TestPorts_Find_AllParams(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Ports.Find(PortFindParams{
		Name:     "ROTTERDAM",
		UUID:     "p-1",
		Fuzzy:    intPtr(1),
		PortType: "seaport",
		Unlocode: "NLRTM",
		Lat:      floatPtr(51.95),
		Lon:      floatPtr(4.14),
		Radius:   floatPtr(10),
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	for _, k := range []string{"name", "uuid", "fuzzy", "port_type", "unlocode", "lat", "lon", "radius"} {
		if cap.query.Get(k) == "" {
			t.Fatalf("param %q not sent: %v", k, cap.query)
		}
	}
}

func TestVessels_Find_FloatRangeCounts(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	if _, err := c.Vessels.Find(VesselFindParams{LengthMax: floatPtr(400)}); err != nil {
		t.Fatalf("float range bound should satisfy guard: %v", err)
	}
	if cap.query.Get("length_max") != "400" {
		t.Fatalf("length_max missing: %v", cap.query)
	}
}

func TestPost_ConnectionError_IsAPIError(t *testing.T) {
	c, err := NewClient("k", withBaseURLs("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Reports.Submit(ReportSubmitParams{ReportType: "x"}); err == nil {
		t.Fatal("expected connection error on POST")
	} else {
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Fatalf("expected APIError, got %T", err)
		}
	}
}

// --- Routes path fix ---

func TestRoutes_Calculate_CorrectPath(t *testing.T) {
	cap := &capture{}
	inner := `{"from":{"type":"Feature"},"to":{"type":"Feature"},"route":{"type":"Feature","properties":{"total_dist":500}}}`
	srv := newServer(t, 200, dataEnvelope(inner), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Routes.Calculate(RouteParams{
		PortUUIDFrom: "p-1",
		PortUUIDTo:   "p-2",
	})
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if cap.path != "/route" {
		t.Fatalf("expected /route, got %s", cap.path)
	}
}

// --- Key redaction in error messages ---

func TestConnectionError_KeyNotLeaked(t *testing.T) {
	const secret = "super-secret-key-12345"
	c, err := NewClient(secret, withBaseURLs("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Stat()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("API key leaked in error: %v", err)
	}
}

// --- InRadiusHistory ---

func TestReports_InRadiusHistory_HappyPath(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"report_id":"r-1","report_type":"inradius_history","status":"_PENDING_"}`), cap)
	c := mustClient(t, srv.URL)

	rep, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{
		Lat:    floatPtr(51.89),
		Lon:    floatPtr(4.39),
		Radius: 10,
		From:   "2024-01-01",
		To:     "2024-01-07",
	})
	if err != nil {
		t.Fatalf("InRadiusHistory: %v", err)
	}
	if rep.ReportID != "r-1" || rep.Status != "_PENDING_" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if cap.method != "POST" {
		t.Fatalf("expected POST, got %s", cap.method)
	}
	if cap.body["report_type"] != "inradius_history" {
		t.Fatalf("report_type missing: %v", cap.body)
	}
	if cap.body["lat"] != 51.89 || cap.body["lon"] != 4.39 {
		t.Fatalf("lat/lon missing: %v", cap.body)
	}
}

func TestReports_InRadiusHistory_ByPort(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`{"report_id":"r-2","report_type":"inradius_history","status":"_PENDING_"}`), cap)
	c := mustClient(t, srv.URL)

	_, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{
		PortUnlocode: "NLRTM",
		Radius:       25,
		From:         "2024-03-01",
		To:           "2024-03-10",
	})
	if err != nil {
		t.Fatalf("InRadiusHistory by port: %v", err)
	}
	if cap.body["port_unlocode"] != "NLRTM" {
		t.Fatalf("port_unlocode missing: %v", cap.body)
	}
}

func TestReports_InRadiusHistory_MissingCenter(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{Radius: 10, From: "2024-01-01", To: "2024-01-07"})
	if err == nil {
		t.Fatal("expected error for missing center")
	}
	var de *DatalasticError
	if !errors.As(err, &de) {
		t.Fatalf("expected DatalasticError, got %T", err)
	}
}

func TestReports_InRadiusHistory_MissingRadius(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{Lat: floatPtr(1), Lon: floatPtr(2), From: "2024-01-01", To: "2024-01-07"})
	if err == nil {
		t.Fatal("expected error for missing radius")
	}
}

func TestReports_InRadiusHistory_MissingDates(t *testing.T) {
	srv := newServer(t, 200, dataEnvelope(`{}`), nil)
	c := mustClient(t, srv.URL)
	_, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 10})
	if err == nil {
		t.Fatal("expected error for missing dates")
	}
}

// --- PortFind radius guard ---

func TestPorts_Find_RadiusWithoutLatLon_NotSent(t *testing.T) {
	cap := &capture{}
	srv := newServer(t, 200, dataEnvelope(`[]`), cap)
	c := mustClient(t, srv.URL)
	_, err := c.Ports.Find(PortFindParams{CountryISO: "NL", Radius: floatPtr(50)})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cap.query.Get("radius") != "" {
		t.Fatalf("radius should not be sent without lat/lon, got: %v", cap.query)
	}
}
