package geodata

import (
	"context"
	"net"

	"github.com/xtls/xray-core/common/errors"
)

type mmdbIPMatcher struct {
	file    string
	code    string
	reverse bool
}

func newMMDBIPMatcher(file, code string, reverse bool) *mmdbIPMatcher {
	return &mmdbIPMatcher{
		file:    file,
		code:    code,
		reverse: reverse,
	}
}

// match checks if the ip belongs to the country code in mmdb
func (m *mmdbIPMatcher) Match(ip net.IP) bool {
	if ip == nil {
		return false
	}
	matched, err := lookupMMDB(m.file, ip, m.code)
	if err != nil {
		errors.LogError(context.Background(), "mmdbIPMatcher.Match: ", err)
		return false
	}
	if m.reverse {
		return !matched
	}
	return matched
}

// anymatch returns true if at least one ip matches
func (m *mmdbIPMatcher) AnyMatch(ips []net.IP) bool {
	for _, ip := range ips {
		if m.Match(ip) {
			return true
		}
	}
	return false
}

// matches returns true only if ALL ips match
func (m *mmdbIPMatcher) Matches(ips []net.IP) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !m.Match(ip) {
			return false
		}
	}
	return true
}

// filterips splits ips into matched and unmatched slices
func (m *mmdbIPMatcher) FilterIPs(ips []net.IP) (matched []net.IP, unmatched []net.IP) {
	for _, ip := range ips {
		if m.Match(ip) {
			matched = append(matched, ip)
		} else {
			unmatched = append(unmatched, ip)
		}
	}
	return
}

// togglereverse flips the reverse flag
func (m *mmdbIPMatcher) ToggleReverse() {
	m.reverse = !m.reverse
}

// setreverse sets the reverse flag directly
func (m *mmdbIPMatcher) SetReverse(reverse bool) {
	m.reverse = reverse
}
