// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type ipaddresses struct {
	Version      int          `yaml:"version"`
	IPAddresses_ []*ipaddress `yaml:"ip-addresses"`
}

type ipaddress struct {
	ProviderID_       string   `yaml:"provider-id,omitempty"`
	DeviceName_       string   `yaml:"device-name"`
	MachineID_        string   `yaml:"machine-id"`
	SubnetCIDR_       string   `yaml:"subnet-cidr"`
	ConfigMethod_     string   `yaml:"config-method"`
	Value_            string   `yaml:"value"`
	DNSServers_       []string `yaml:"dns-servers"`
	DNSSearchDomains_ []string `yaml:"dns-search-domains"`
	GatewayAddress_   string   `yaml:"gateway-address"`
}

// ProviderID implements IPAddress.
func (i *ipaddress) ProviderID() string {
	return i.ProviderID_
}

// DeviceName implements IPAddress.
func (i *ipaddress) DeviceName() string {
	return i.DeviceName_
}

// MachineID implements IPAddress.
func (i *ipaddress) MachineID() string {
	return i.MachineID_
}

// SubnetCIDR implements IPAddress.
func (i *ipaddress) SubnetCIDR() string {
	return i.SubnetCIDR_
}

// ConfigMethod implements IPAddress.
func (i *ipaddress) ConfigMethod() string {
	return i.ConfigMethod_
}

// Value implements IPAddress.
func (i *ipaddress) Value() string {
	return i.Value_
}

// DNSServers implements IPAddress.
func (i *ipaddress) DNSServers() []string {
	return i.DNSServers_
}

// DNSSearchDomains implements IPAddress.
func (i *ipaddress) DNSSearchDomains() []string {
	return i.DNSSearchDomains_
}

// GatewayAddress implements IPAddress.
func (i *ipaddress) GatewayAddress() string {
	return i.GatewayAddress_
}

// IPAddressArgs is an argument struct used to create a
// new internal ipaddress type that supports the IPAddress interface.
type IPAddressArgs struct {
	ProviderID       string
	DeviceName       string
	MachineID        string
	SubnetCIDR       string
	ConfigMethod     string
	Value            string
	DNSServers       []string
	DNSSearchDomains []string
	GatewayAddress   string
}

func newIPAddress(args IPAddressArgs) *ipaddress {
	return &ipaddress{
		ProviderID_:       args.ProviderID,
		DeviceName_:       args.DeviceName,
		MachineID_:        args.MachineID,
		SubnetCIDR_:       args.SubnetCIDR,
		ConfigMethod_:     args.ConfigMethod,
		Value_:            args.Value,
		DNSServers_:       args.DNSServers,
		DNSSearchDomains_: args.DNSSearchDomains,
		GatewayAddress_:   args.GatewayAddress,
	}
}

func importIPAddresses(source map[string]interface{}) ([]*ipaddress, error) {
	checker := versionedChecker("ip-addresses")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ip-addresses version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := ipaddressDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["ip-addresses"].([]interface{})
	return importIPAddressList(sourceList, importFunc)
}

func importIPAddressList(sourceList []interface{}, importFunc ipaddressDeserializationFunc) ([]*ipaddress, error) {
	result := make([]*ipaddress, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for ip-address %d, %T", i, value)
		}
		ipaddress, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "ip-address %d", i)
		}
		result = append(result, ipaddress)
	}
	return result, nil
}

type ipaddressDeserializationFunc func(map[string]interface{}) (*ipaddress, error)

var ipaddressDeserializationFuncs = map[int]ipaddressDeserializationFunc{
	1: importIPAddressV1,
}

func importIPAddressV1(source map[string]interface{}) (*ipaddress, error) {
	fields := schema.Fields{
		"provider-id":        schema.String(),
		"device-name":        schema.String(),
		"machine-id":         schema.String(),
		"subnet-cidr":        schema.String(),
		"config-method":      schema.String(),
		"value":              schema.String(),
		"dns-servers":        schema.List(schema.String()),
		"dns-search-domains": schema.List(schema.String()),
		"gateway-address":    schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ip address v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	dnsserversInterface := valid["dns-servers"].([]interface{})
	dnsservers := make([]string, len(dnsserversInterface))
	for i, d := range dnsserversInterface {
		dnsservers[i] = d.(string)
	}
	dnssearchInterface := valid["dns-search-domains"].([]interface{})
	dnssearch := make([]string, len(dnssearchInterface))
	for i, d := range dnssearchInterface {
		dnssearch[i] = d.(string)
	}
	return &ipaddress{
		ProviderID_:       valid["provider-id"].(string),
		DeviceName_:       valid["device-name"].(string),
		MachineID_:        valid["machine-id"].(string),
		SubnetCIDR_:       valid["subnet-cidr"].(string),
		ConfigMethod_:     valid["config-method"].(string),
		Value_:            valid["value"].(string),
		DNSServers_:       dnsservers,
		DNSSearchDomains_: dnssearch,
		GatewayAddress_:   valid["gateway-address"].(string),
	}, nil
}
