package geodata

import (
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/platform/filesystem"
)

// cache for mmdb readers so we dont open the file every single time
var (
	mmdbCache   = make(map[string]*maxminddb.Reader)
	mmdbCacheMu sync.Mutex
)

// struct for decoding the country record from mmdb
// maxminddb uses these tags to map fields
// registered_country is a fallback for some geoip databases
type mmdbCountryRecord struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"registered_country"`
}

func (r *mmdbCountryRecord) isoCode() string {
	if r.Country.IsoCode != "" {
		return r.Country.IsoCode
	}
	return r.RegisteredCountry.IsoCode
}

// struct for decoding asn records
type mmdbASNRecord struct {
	ASN          uint32 `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
}

// opens the mmdb file and caches the reader
func openMMDB(file string) (*maxminddb.Reader, error) {
	mmdbCacheMu.Lock()
	defer mmdbCacheMu.Unlock()

	if reader, ok := mmdbCache[file]; ok {
		return reader, nil
	}

	path, err := filesystem.ResolveAsset(file)
	if err != nil {
		return nil, errors.New("failed to resolve asset path for ", file).Base(err)
	}

	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, errors.New("failed to open mmdb file ", file).Base(err)
	}

	mmdbCache[file] = reader
	return reader, nil
}

// close all cached mmdb readers, usefull for reload
func closeAllMMDB() {
	mmdbCacheMu.Lock()
	defer mmdbCacheMu.Unlock()

	for _, reader := range mmdbCache {
		reader.Close()
	}
	mmdbCache = make(map[string]*maxminddb.Reader)
}

// check if code is an asn code like AS15169
func isASNCode(code string) (uint32, bool) {
	upper := strings.ToUpper(code)
	if !strings.HasPrefix(upper, "AS") {
		return 0, false
	}
	numStr := upper[2:]
	if numStr == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(numStr, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// look up an ip in the mmdb and check if it matches the country or asn code
func lookupMMDB(file string, ip net.IP, code string) (bool, error) {
	reader, err := openMMDB(file)
	if err != nil {
		return false, err
	}

	// try asn lookup first if code looks like AS12345
	if asnNum, ok := isASNCode(code); ok {
		var record mmdbASNRecord
		if err := reader.Lookup(ip, &record); err != nil {
			return false, errors.New("failed to lookup IP in asn mmdb ", file).Base(err)
		}
		return record.ASN == asnNum, nil
	}

	var record mmdbCountryRecord
	if err := reader.Lookup(ip, &record); err != nil {
		return false, errors.New("failed to lookup IP in mmdb ", file).Base(err)
	}

	return strings.EqualFold(record.isoCode(), code), nil
}
