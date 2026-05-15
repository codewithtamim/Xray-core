package geodata

import (
	"net"
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
type mmdbCountryRecord struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
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

// look up an ip in the mmdb and check if it matches the country code
func lookupMMDB(file string, ip net.IP, code string) (bool, error) {
	reader, err := openMMDB(file)
	if err != nil {
		return false, err
	}

	var record mmdbCountryRecord
	if err := reader.Lookup(ip, &record); err != nil {
		return false, errors.New("failed to lookup IP in mmdb ", file).Base(err)
	}

	return strings.EqualFold(record.Country.IsoCode, code), nil
}
