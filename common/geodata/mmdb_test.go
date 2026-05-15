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
		{"1.1.1.1", "CLOUDFLARE", true},
		{"1.1.1.1", "CN", false},
		{"8.8.8.8", "GOOGLE", true},
		{"128.101.101.101", "US", true},
		{"::1", "US", false},
	}

	for _, c := range cases {
		matched, err := lookupMMDB("Country.mmdb", net.ParseIP(c.ip), c.code)
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

	m := newMMDBIPMatcher("Country.mmdb", "CN", false)

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

	if err := checkFile("Country.mmdb", "CN"); err != nil {
		t.Fatalf("checkFile for Country.mmdb failed: %v", err)
	}
	if err := checkFile("Country.mmdb", "US"); err != nil {
		t.Fatalf("checkFile for Country.mmdb failed: %v", err)
	}
	if err := checkFile("nonexistent.mmdb", "CN"); err == nil {
		t.Fatal("expected checkFile to fail for nonexistent.mmdb")
	}
}

func TestMMDBBuildOptimizedIPMatcher(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules := []*IPRule{
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "Country.mmdb", Code: "CN", ReverseMatch: false}}},
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "Country.mmdb", Code: "US", ReverseMatch: true}}},
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
	_, err := lookupMMDB("Country.mmdb", net.ParseIP("1.1.1.1"), "US")
	common.Must(err)

	closeAllMMDB()

	// after closing it should still work cuz we reopen
	_, err = lookupMMDB("Country.mmdb", net.ParseIP("1.1.1.1"), "US")
	common.Must(err)
}

func TestMMDBMixedWithCustomCIDR(t *testing.T) {
	t.Setenv("xray.location.asset", "testdata")

	rules := []*IPRule{
		{Value: &IPRule_Custom{Custom: &CIDRRule{Cidr: &CIDR{Ip: []byte{192, 168, 0, 0}, Prefix: 16}, ReverseMatch: false}}},
		{Value: &IPRule_Geoip{Geoip: &GeoIPRule{File: "Country.mmdb", Code: "CN", ReverseMatch: false}}},
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
		"ext:Country.mmdb:CN",
		"!ext:Country.mmdb:US",
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

func TestMain(m *testing.M) {
	// set default asset location for tests that dont call t.Setenv
	os.Setenv("xray.location.asset", filepath.Join("testdata"))
	os.Exit(m.Run())
}
