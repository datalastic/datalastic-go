package datalastic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// envelopeWithMeta wraps a data payload and a literal meta object.
func envelopeWithMeta(data, meta string) string {
	return `{"data":` + data + `,"meta":` + meta + `}`
}

// --- Meta parsing ---

func TestMeta_Unmarshal(t *testing.T) {
	cases := []struct {
		name     string
		meta     string
		success  *bool
		message  string
		next     string
		endpoint string
		duration float64
		rawKeys  []string
	}{
		{
			name:     "documented keys",
			meta:     `{"success":true,"message":"ok","next":"tok","endpoint":"/vessel","duration":0.25}`,
			success:  boolPtr(true),
			message:  "ok",
			next:     "tok",
			endpoint: "/vessel",
			duration: 0.25,
			rawKeys:  []string{"success", "message", "next", "endpoint", "duration"},
		},
		{
			name:     "unmodelled keys survive in raw",
			meta:     `{"success":true,"credits_remaining":9812,"plan":"pro"}`,
			success:  boolPtr(true),
			duration: 0,
			rawKeys:  []string{"success", "credits_remaining", "plan"},
		},
		{
			name:     "duration as a numeric string",
			meta:     `{"duration":"1.75"}`,
			duration: 1.75,
			rawKeys:  []string{"duration"},
		},
		{
			name:     "unparseable duration is ignored",
			meta:     `{"duration":"fast"}`,
			duration: 0,
			rawKeys:  []string{"duration"},
		},
		{
			name:    "success as a quoted boolean",
			meta:    `{"success":"false"}`,
			success: boolPtr(false),
			rawKeys: []string{"success"},
		},
		{
			name:    "unparseable success stays nil",
			meta:    `{"success":17}`,
			rawKeys: []string{"success"},
		},
		{
			name:    "non-string message is ignored",
			meta:    `{"message":42}`,
			rawKeys: []string{"message"},
		},
		{
			name:    "empty object",
			meta:    `{}`,
			rawKeys: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m Meta
			if err := json.Unmarshal([]byte(tc.meta), &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			switch {
			case tc.success == nil && m.Success != nil:
				t.Fatalf("Success = %v, want nil", *m.Success)
			case tc.success != nil && m.Success == nil:
				t.Fatalf("Success = nil, want %v", *tc.success)
			case tc.success != nil && *m.Success != *tc.success:
				t.Fatalf("Success = %v, want %v", *m.Success, *tc.success)
			}
			if m.Message != tc.message {
				t.Fatalf("Message = %q, want %q", m.Message, tc.message)
			}
			if m.Next != tc.next {
				t.Fatalf("Next = %q, want %q", m.Next, tc.next)
			}
			if m.Endpoint != tc.endpoint {
				t.Fatalf("Endpoint = %q, want %q", m.Endpoint, tc.endpoint)
			}
			if m.Duration != tc.duration {
				t.Fatalf("Duration = %v, want %v", m.Duration, tc.duration)
			}
			if m.Raw == nil {
				t.Fatal("Raw must be non-nil for a meta object")
			}
			if len(m.Raw) != len(tc.rawKeys) {
				t.Fatalf("Raw has %d keys, want %d: %v", len(m.Raw), len(tc.rawKeys), m.Raw)
			}
			for _, k := range tc.rawKeys {
				if _, ok := m.Raw[k]; !ok {
					t.Fatalf("Raw missing key %q: %v", k, m.Raw)
				}
			}
		})
	}
}

func TestMeta_NotAnObjectIsAnError(t *testing.T) {
	var m Meta
	err := json.Unmarshal([]byte(`"ok"`), &m)
	if err == nil {
		t.Fatal("expected an error for a non-object meta")
	}
}

func TestMeta_AbsentYieldsEmptyMeta(t *testing.T) {
	srv := newServer(t, 200, `{"data":{"user_id":"u1"}}`, nil)
	c := mustClient(t, srv.URL)

	stat, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Meta == nil {
		t.Fatal("Meta must be non-nil even without a meta object")
	}
	if stat.Meta.Success != nil {
		t.Fatalf("Success = %v, want nil", *stat.Meta.Success)
	}
	if len(stat.Meta.Raw) != 0 {
		t.Fatalf("Raw = %v, want empty", stat.Meta.Raw)
	}
}

func TestMeta_RawExposesUnmodelledCounters(t *testing.T) {
	body := envelopeWithMeta(`{"user_id":"u1"}`, `{"success":true,"credits_remaining":9812}`)
	srv := newServer(t, 200, body, nil)
	c := mustClient(t, srv.URL)

	stat, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	raw, ok := stat.Meta.Raw["credits_remaining"]
	if !ok {
		t.Fatalf("credits_remaining missing from Raw: %v", stat.Meta.Raw)
	}
	var credits int
	if err := json.Unmarshal(raw, &credits); err != nil {
		t.Fatalf("decode credits_remaining: %v", err)
	}
	if credits != 9812 {
		t.Fatalf("credits_remaining = %d, want 9812", credits)
	}
}

// --- HTTP 200 with meta.success false ---

func TestMeta_SuccessFalse(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantMsg string
	}{
		{
			name:    "success false with message and no data",
			body:    `{"meta":{"success":false,"message":"vessel not in coverage"}}`,
			wantErr: true,
			wantMsg: "API reported failure: vessel not in coverage",
		},
		{
			name:    "success false with data present",
			body:    envelopeWithMeta(`{"user_id":"u1"}`, `{"success":false,"message":"stale"}`),
			wantErr: true,
			wantMsg: "API reported failure: stale",
		},
		{
			name:    "success false without a message",
			body:    envelopeWithMeta(`{"user_id":"u1"}`, `{"success":false}`),
			wantErr: true,
			wantMsg: "API reported failure",
		},
		{
			name:    "success key absent with data present",
			body:    envelopeWithMeta(`{"user_id":"u1"}`, `{"endpoint":"/stat"}`),
			wantErr: false,
		},
		{
			name:    "meta absent entirely",
			body:    `{"data":{"user_id":"u1"}}`,
			wantErr: false,
		},
		{
			name:    "success true",
			body:    envelopeWithMeta(`{"user_id":"u1"}`, `{"success":true}`),
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, 200, tc.body, nil)
			c := mustClient(t, srv.URL)

			stat, err := c.Stat()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if stat.UserID != "u1" {
					t.Fatalf("unexpected stat: %+v", stat)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error, got stat %+v", stat)
			}
			if stat != nil {
				t.Fatalf("expected a nil result alongside the error, got %+v", stat)
			}
			var ae *APIError
			if !errors.As(err, &ae) {
				t.Fatalf("expected APIError, got %T", err)
			}
			if ae.StatusCode != 200 {
				t.Fatalf("StatusCode = %d, want 200", ae.StatusCode)
			}
			if ae.Message != tc.wantMsg {
				t.Fatalf("message = %q, want %q", ae.Message, tc.wantMsg)
			}
		})
	}
}

func TestMeta_SuccessFalseOnFourOhFourStaysTyped(t *testing.T) {
	// Status-code mapping runs first: a 404 is a NotFoundError regardless of
	// what the meta object claims.
	srv := newServer(t, 404, `{"meta":{"success":false,"message":"gone"},"data":{"message":"gone"}}`, nil)
	c := mustClient(t, srv.URL)

	_, err := c.Stat()
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
}

// --- Meta on returned values ---

func TestMeta_PopulatedOnEveryReturnedType(t *testing.T) {
	const metaJSON = `{"success":true,"endpoint":"/test","duration":0.5,"next":"tok"}`

	cases := []struct {
		name string
		data string
		call func(c *Client) (*Meta, error)
	}{
		{"Stat", `{"user_id":"u1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Stat()
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.Get", `{"uuid":"v-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Get(VesselParams{IMO: "9999"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.Pro", `{"uuid":"v-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Pro(VesselParams{IMO: "9999"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil // promoted from the embedded Vessel
		}},
		{"Vessels.Estimated", `{"uuid":"v-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Estimated(VesselParams{IMO: "9999"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil // promoted through VesselPro
		}},
		{"Vessels.Bulk", `{"total":1,"vessels":[{"uuid":"a"}]}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Bulk(VesselBulkParams{MMSI: []string{"1"}})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.InRadius", `{"point":{"lat":1,"lon":2,"radius":5},"total":0,"vessels":[]}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 5})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.History", `{"uuid":"v-1","positions":[]}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.History(VesselHistoryParams{UUID: "v-1"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.Info", `{"uuid":"v-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Info(VesselParams{IMO: "9999"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Vessels.Find", `[{"uuid":"v-1"}]`, func(c *Client) (*Meta, error) {
			v, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Ports.Get", `{"uuid":"p-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Ports.Get(PortGetParams{Unlocode: "NLRTM"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Routes.Calculate", `{"from":{},"to":{},"route":{}}`, func(c *Client) (*Meta, error) {
			v, err := c.Routes.Calculate(RouteParams{PortUUIDFrom: "a", PortUUIDTo: "b"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Reports.Get", `{"report_id":"r-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Reports.Get("r-1")
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Reports.Submit", `{"report_id":"r-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet"})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
		{"Reports.InRadiusHistory", `{"report_id":"r-1"}`, func(c *Client) (*Meta, error) {
			v, err := c.Reports.InRadiusHistory(InRadiusHistoryParams{
				Lat: floatPtr(1), Lon: floatPtr(2), Radius: 5, From: "2024-01-01", To: "2024-01-02",
			})
			if err != nil {
				return nil, err
			}
			return v.Meta, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, 200, envelopeWithMeta(tc.data, metaJSON), nil)
			c := mustClient(t, srv.URL)

			meta, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if meta == nil {
				t.Fatalf("%s: Meta is nil", tc.name)
			}
			if meta.Endpoint != "/test" || meta.Duration != 0.5 {
				t.Fatalf("%s: unexpected meta %+v", tc.name, meta)
			}
			if meta.Success == nil || !*meta.Success {
				t.Fatalf("%s: Success not carried through", tc.name)
			}
			if len(meta.Raw) != 4 {
				t.Fatalf("%s: Raw = %v, want 4 keys", tc.name, meta.Raw)
			}
		})
	}
}

func TestMeta_VesselWithDistPromotedMetaStaysNil(t *testing.T) {
	data := `{"point":{"lat":1,"lon":2,"radius":5},"total":1,"vessels":[{"uuid":"a","distance":3}]}`
	srv := newServer(t, 200, envelopeWithMeta(data, `{"success":true}`), nil)
	c := mustClient(t, srv.URL)

	res, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 5})
	if err != nil {
		t.Fatalf("InRadius: %v", err)
	}
	if res.Meta == nil {
		t.Fatal("result Meta must be populated")
	}
	if len(res.Vessels) != 1 {
		t.Fatalf("unexpected vessels: %+v", res.Vessels)
	}
	if res.Vessels[0].Meta != nil {
		t.Fatal("the Meta promoted into a slice element must stay nil")
	}
}

func TestMeta_NextMirroredOnPaginatedResults(t *testing.T) {
	t.Run("Find", func(t *testing.T) {
		srv := newServer(t, 200, envelopeWithMeta(`[{"uuid":"v-1"}]`, `{"success":true,"next":"page-2"}`), nil)
		c := mustClient(t, srv.URL)
		res, err := c.Vessels.Find(VesselFindParams{VesselType: "cargo"})
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if res.Next != "page-2" || res.Meta.Next != "page-2" {
			t.Fatalf("Next = %q, Meta.Next = %q, want both page-2", res.Next, res.Meta.Next)
		}
	})
	t.Run("InRadius", func(t *testing.T) {
		data := `{"point":{"lat":1,"lon":2,"radius":5},"total":0,"vessels":[]}`
		srv := newServer(t, 200, envelopeWithMeta(data, `{"success":true,"next":"page-2"}`), nil)
		c := mustClient(t, srv.URL)
		res, err := c.Vessels.InRadius(VesselInRadiusParams{Lat: floatPtr(1), Lon: floatPtr(2), Radius: 5})
		if err != nil {
			t.Fatalf("InRadius: %v", err)
		}
		if res.Next != "page-2" || res.Meta.Next != "page-2" {
			t.Fatalf("Next = %q, Meta.Next = %q, want both page-2", res.Next, res.Meta.Next)
		}
	})
}

// --- WithMeta siblings for slice-returning methods ---

func TestMeta_WithMetaSiblings(t *testing.T) {
	const metaJSON = `{"success":true,"endpoint":"/test","duration":0.5}`

	cases := []struct {
		name string
		data string
		// withMeta returns the number of decoded records and the meta.
		withMeta func(c *Client) (int, *Meta, error)
		// plain returns the number of decoded records from the sibling that
		// drops the meta.
		plain func(c *Client) (int, error)
	}{
		{
			name: "Ports.Find",
			data: `[{"uuid":"p-1"}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Ports.FindWithMeta(PortFindParams{CountryISO: "NL"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Ports.Find(PortFindParams{CountryISO: "NL"})
				return len(v), err
			},
		},
		{
			name: "Intel.DryDock",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.DryDockWithMeta(IntelDryDockParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.DryDock(IntelDryDockParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.Casualties",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.CasualtiesWithMeta(IntelDateRangeParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.Casualties(IntelDateRangeParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.Inspections",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.InspectionsWithMeta(IntelDateRangeParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.Inspections(IntelDateRangeParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.SPD",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.SPDWithMeta(IntelDateRangeParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.SPD(IntelDateRangeParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.Ownership",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.OwnershipWithMeta(OwnershipParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.Ownership(OwnershipParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.ClassSociety",
			data: `[{"imo":"9999"}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.ClassSocietyWithMeta(ClassSocietyParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.ClassSociety(ClassSocietyParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.Engine",
			data: `[{"imo":"9999"}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.EngineWithMeta(EngineParams{IMO: "9999"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.Engine(EngineParams{IMO: "9999"})
				return len(v), err
			},
		},
		{
			name: "Intel.Companies",
			data: `[{"id":1}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Intel.CompaniesWithMeta(CompanyParams{CompanyIMO: "1234567"})
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Intel.Companies(CompanyParams{CompanyIMO: "1234567"})
				return len(v), err
			},
		},
		{
			name: "Reports.ListAll",
			data: `[{"report_id":"r-1"}]`,
			withMeta: func(c *Client) (int, *Meta, error) {
				v, m, err := c.Reports.ListAllWithMeta()
				return len(v), m, err
			},
			plain: func(c *Client) (int, error) {
				v, err := c.Reports.ListAll()
				return len(v), err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, 200, envelopeWithMeta(tc.data, metaJSON), nil)
			c := mustClient(t, srv.URL)

			n, meta, err := tc.withMeta(c)
			if err != nil {
				t.Fatalf("%sWithMeta: %v", tc.name, err)
			}
			if n != 1 {
				t.Fatalf("%sWithMeta returned %d records, want 1", tc.name, n)
			}
			if meta == nil || meta.Endpoint != "/test" || meta.Duration != 0.5 {
				t.Fatalf("%sWithMeta returned meta %+v", tc.name, meta)
			}

			plainN, err := tc.plain(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if plainN != n {
				t.Fatalf("%s returned %d records but its WithMeta sibling returned %d", tc.name, plainN, n)
			}
		})
	}
}

func TestMeta_WithMetaSiblingsPropagateErrors(t *testing.T) {
	srv := newServer(t, 401, `{"data":{"message":"nope"}}`, nil)
	c := mustClient(t, srv.URL)

	if _, _, err := c.Ports.FindWithMeta(PortFindParams{CountryISO: "NL"}); err == nil {
		t.Fatal("expected an error from FindWithMeta")
	} else {
		var ae *AuthenticationError
		if !errors.As(err, &ae) {
			t.Fatalf("expected AuthenticationError, got %T", err)
		}
	}

	// The validation guard must fire before any request and still return the
	// three-value signature cleanly.
	ports, meta, err := c.Ports.FindWithMeta(PortFindParams{Fuzzy: intPtr(1)})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if ports != nil || meta != nil {
		t.Fatalf("expected nil results on validation failure, got %v %v", ports, meta)
	}
}

func TestMeta_WithMetaSiblingDecodeFailure(t *testing.T) {
	// data is an object where a slice is expected.
	srv := newServer(t, 200, envelopeWithMeta(`{"id":1}`, `{"success":true}`), nil)
	c := mustClient(t, srv.URL)

	records, meta, err := c.Intel.DryDockWithMeta(IntelDryDockParams{IMO: "9999"})
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if records != nil || meta != nil {
		t.Fatalf("expected nil results on decode failure, got %v %v", records, meta)
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if !strings.Contains(ae.Message, "dry_dock_dates") {
		t.Fatalf("message should name what failed to decode, got %q", ae.Message)
	}
}
