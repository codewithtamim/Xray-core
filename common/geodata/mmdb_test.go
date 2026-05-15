package geodata

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/xtls/xray-core/common"
)

func TestMMDBLookup(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	cases := []struct {
		ip      string
		code    string
		matched bool
	}{
		{"223.5.5.5", "CN", true},
		{"1.1.1.1", "AU", true},
		{"1.1.1.1", "CN", false},
		{"8.8.8.8", "US", true},
		{"128.101.101.101", "US", true},
		{"::1", "US", false},
	}

	for _, c := range cases {
		matched, err := lookupMMDB("GeoLite2-Country.mmdb", net.ParseIP(c.ip), c.code)
		if err != nil {
			t.Fatalf("lookupMMDB(%s, %s) error: %v", c.ip, c.code, err)
		}
		if matched != c.matched {
			t.Fatalf("lookupMMDB(%s, %s) = %v, want %v", c.ip, c.code, matched, c.matched)
		}
	}
}

func TestMMDBIPMatcher(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	m := newMMDBIPMatcher("GeoLite2-Country.mmdb", "CN", false)

	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match CN")
	}
	if m.Match(net.ParseIP("128.101.101.101")) {
		t.Fatal("expected 128.101.101.101 not to match CN")
	}
	if m.Match(nil) {
		t.Fatal("expected nil IP not to match")
	}

	// anymatch tests
	if !m.AnyMatch([]net.IP{net.ParseIP("223.5.5.5"), net.ParseIP("128.101.101.101")}) {
		t.Fatal("expected AnyMatch true when one IP matches")
	}
	if m.AnyMatch([]net.IP{net.ParseIP("128.101.101.101")}) {
		t.Fatal("expected AnyMatch false when no IP matches")
	}

	// matches tests
	if !m.Matches([]net.IP{net.ParseIP("223.5.5.5")}) {
		t.Fatal("expected Matches true for single matching IP")
	}
	if m.Matches([]net.IP{net.ParseIP("223.5.5.5"), net.ParseIP("128.101.101.101")}) {
		t.Fatal("expected Matches false when one IP does not match")
	}
	if m.Matches(nil) {
		t.Fatal("expected Matches false for nil slice")
	}

	// filterips tests
	matched, unmatched := m.FilterIPs([]net.IP{net.ParseIP("223.5.5.5"), net.ParseIP("128.101.101.101")})
	if len(matched) != 1 || len(unmatched) != 1 {
		t.Fatalf("expected 1 matched and 1 unmatched, got %d and %d", len(matched), len(unmatched))
	}

	// reverse tests
	m.SetReverse(true)
	if m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 not to match reverse CN")
	}
	if !m.Match(net.ParseIP("128.101.101.101")) {
		t.Fatal("expected 128.101.101.101 to match reverse CN")
	}

	m.ToggleReverse()
	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match after toggle reverse back")
	}
}

func TestMMDBCheckFile(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	if err := checkFile("GeoLite2-Country.mmdb", "CN"); err != nil {
		t.Fatalf("checkFile for GeoLite2-Country.mmdb failed: %v", err)
	}
	if err := checkFile("GeoLite2-Country.mmdb", "US"); err != nil {
		t.Fatalf("checkFile for GeoLite2-Country.mmdb failed: %v", err)
	}
	if err := checkFile("nonexistent.mmdb", "CN"); err == nil {
		t.Fatal("expected checkFile to fail for nonexistent.mmdb")
	}
}

func TestMMDBBuildOptimizedIPMatcher(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules := []*IPRule{
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "GeoLite2-Country.mmdb", Code: "CN", ReverseMatch: false}}},
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "GeoLite2-Country.mmdb", Code: "US", ReverseMatch: true}}},
	}

	m, err := buildOptimizedIPMatcher(newIPSetFactory(), rules)
	common.Must(err)

	// cn should match
	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match")
	}
	// us is reversed so us ips should NOT match
	if m.Match(net.ParseIP("128.101.101.101")) {
		t.Fatal("expected 128.101.101.101 not to match because US is reversed")
	}
	// non-us non-cn should match because of reversed us rule
	if !m.Match(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected 1.1.1.1 to match because it is not US")
	}
}

func TestMMDBCloseAll(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	// make sure cache has something
	_, err := lookupMMDB("GeoLite2-Country.mmdb", net.ParseIP("1.1.1.1"), "US")
	common.Must(err)

	closeAllMMDB()

	// after closing it should still work cuz we reopen
	_, err = lookupMMDB("GeoLite2-Country.mmdb", net.ParseIP("1.1.1.1"), "US")
	common.Must(err)
}

func TestMMDBMixedWithCustomCIDR(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules := []*IPRule{
		{Value: &IPRule_Custom{Custom: &CIDRRule{Cidr: &CIDR{Ip: []byte{192, 168, 0, 0}, Prefix: 16}, ReverseMatch: false}}},
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "GeoLite2-Country.mmdb", Code: "CN", ReverseMatch: false}}},
	}

	m, err := buildOptimizedIPMatcher(newIPSetFactory(), rules)
	common.Must(err)

	// custom cidr should still work
	if !m.Match(net.ParseIP("192.168.1.1")) {
		t.Fatal("expected 192.168.1.1 to match custom CIDR")
	}
	// mmdb match
	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match CN via MMDB")
	}
	// neither should match
	if m.Match(net.ParseIP("128.101.101.101")) {
		t.Fatal("expected 128.101.101.101 not to match")
	}
}

func TestMMDBParseIPRules(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules, err := ParseIPRules([]string{
		"ext:GeoLite2-Country.mmdb:CN",
		"!ext:GeoLite2-Country.mmdb:US",
		"192.168.0.0/16",
	})
	common.Must(err)

	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	m, err := newIPRegistry().BuildIPMatcher(rules)
	common.Must(err)

	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match CN via MMDB")
	}
	if m.Match(net.ParseIP("128.101.101.101")) {
		t.Fatal("expected 128.101.101.101 not to match (US is reversed)")
	}
	if !m.Match(net.ParseIP("192.168.1.1")) {
		t.Fatal("expected 192.168.1.1 to match custom CIDR")
	}
}

func TestMMDBASNLookup(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	cases := []struct {
		ip      string
		code    string
		matched bool
	}{
		{"8.8.8.8", "AS15169", true},
		{"1.1.1.1", "AS13335", true},
		{"223.5.5.5", "AS45102", true},
		{"1.1.1.1", "AS15169", false},
		{"8.8.8.8", "AS13335", false},
		{"128.101.101.101", "AS217", true},
	}

	for _, c := range cases {
		matched, err := lookupMMDB("GeoLite2-ASN.mmdb", net.ParseIP(c.ip), c.code)
		if err != nil {
			t.Fatalf("lookupMMDB(%s, %s) error: %v", c.ip, c.code, err)
		}
		if matched != c.matched {
			t.Fatalf("lookupMMDB(%s, %s) = %v, want %v", c.ip, c.code, matched, c.matched)
		}
	}
}

func TestMMDBASNMatcher(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	m := newMMDBIPMatcher("GeoLite2-ASN.mmdb", "AS15169", false)

	if !m.Match(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected 8.8.8.8 to match AS15169")
	}
	if m.Match(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected 1.1.1.1 not to match AS15169")
	}

	// reverse asn
	m.SetReverse(true)
	if m.Match(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected 8.8.8.8 not to match reverse AS15169")
	}
	if !m.Match(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected 1.1.1.1 to match reverse AS15169")
	}
}

func TestMMDBMixedCountryAndASN(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules := []*IPRule{
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "GeoLite2-Country.mmdb", Code: "CN", ReverseMatch: false}}},
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "GeoLite2-ASN.mmdb", Code: "AS15169", ReverseMatch: false}}},
	}

	m, err := buildOptimizedIPMatcher(newIPSetFactory(), rules)
	common.Must(err)

	// CN match
	if !m.Match(net.ParseIP("223.5.5.5")) {
		t.Fatal("expected 223.5.5.5 to match CN")
	}
	// ASN match
	if !m.Match(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected 8.8.8.8 to match AS15169")
	}
	// neither
	if m.Match(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected 1.1.1.1 not to match")
	}
}

func TestMain(m *testing.M) {
	// set default asset location for tests that dont call t.Setenv
	os.Setenv("xray.location.asset", filepath.Join("testdata"))
	os.Exit(m.Run())
}
