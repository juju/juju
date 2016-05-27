// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type ipaddresses struct {
	Version      int          `yaml:"version"`
	IPAddresses_ []*ipaddress `yaml:"ipaddresses"`
}

type ipaddress struct {
	ProviderID_       string
	DeviceName_       string
	MachineID_        string
	SubnetCIDR_       string
	ConfigMethod_     string
	Value_            string
	DNSServers_       []string
	DNSSearchDomains_ []string
	GatewayAddress_   string
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
	checker := versionedChecker("ipaddresses")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ipaddresses version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := ipaddressDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["ipaddresses"].([]interface{})
	return importIPAddressList(sourceList, importFunc)
}

func importIPAddressList(sourceList []interface{}, importFunc ipaddressDeserializationFunc) ([]*ipaddress, error) {
	result := make([]*ipaddress, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for ipaddress %d, %T", i, value)
		}
		ipaddress, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "ipaddress %d", i)
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
		"provider-id":      schema.String(),
		"devicename":       schema.String(),
		"machineid":        schema.String(),
		"subnetcidr":       schema.String(),
		"configmethod":     schema.String(),
		"value":            schema.String(),
		"dnsservers":       schema.List(schema.String()),
		"dnssearchdomains": schema.List(schema.String()),
		"gatewayaddress":   schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ipaddress v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	return &ipaddress{
		ProviderID_:       valid["provider-id"].(string),
		DeviceName_:       valid["devicename"].(string),
		MachineID_:        valid["machinename"].(string),
		SubnetCIDR_:       valid["subnetcidr"].(string),
		ConfigMethod_:     valid["configmethod"].(string),
		Value_:            valid["value"].(string),
		DNSServers_:       valid["dnsservers"].([]string),
		DNSSearchDomains_: valid["dnssearchdomains"].([]string),
		GatewayAddress_:   valid["gatewayaddress"].(string),
	}, nil
}
