// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type sshhostkeys struct {
	Version      int          `yaml:"version"`
	SSHHostKeys_ []*sshhostkey `yaml:"sshhostkeys"`
}

type sshhostkey struct {
	ProviderID_       string   `yaml:"provider-id,omitempty"`
	DeviceName_       string   `yaml:"devicename"`
	MachineID_        string   `yaml:"machineid"`
	SubnetCIDR_       string   `yaml:"subnetcidr"`
	ConfigMethod_     string   `yaml:"configmethod"`
	Value_            string   `yaml:"value"`
	DNSServers_       []string `yaml:"dnsservers"`
	DNSSearchDomains_ []string `yaml:"dnssearchdomains"`
	GatewayAddress_   string   `yaml:"gatewayaddress"`
}

// ProviderID implements SSHHostKey.
func (i *sshhostkey) ProviderID() string {
	return i.ProviderID_
}

// DeviceName implements SSHHostKey.
func (i *sshhostkey) DeviceName() string {
	return i.DeviceName_
}

// MachineID implements SSHHostKey.
func (i *sshhostkey) MachineID() string {
	return i.MachineID_
}

// SubnetCIDR implements SSHHostKey.
func (i *sshhostkey) SubnetCIDR() string {
	return i.SubnetCIDR_
}

// ConfigMethod implements SSHHostKey.
func (i *sshhostkey) ConfigMethod() string {
	return i.ConfigMethod_
}

// Value implements SSHHostKey.
func (i *sshhostkey) Value() string {
	return i.Value_
}

// DNSServers implements SSHHostKey.
func (i *sshhostkey) DNSServers() []string {
	return i.DNSServers_
}

// DNSSearchDomains implements SSHHostKey.
func (i *sshhostkey) DNSSearchDomains() []string {
	return i.DNSSearchDomains_
}

// GatewayAddress implements SSHHostKey.
func (i *sshhostkey) GatewayAddress() string {
	return i.GatewayAddress_
}

// SSHHostKeyArgs is an argument struct used to create a
// new internal sshhostkey type that supports the SSHHostKey interface.
type SSHHostKeyArgs struct {
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

func newSSHHostKey(args SSHHostKeyArgs) *sshhostkey {
	return &sshhostkey{
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

func importSSHHostKeys(source map[string]interface{}) ([]*sshhostkey, error) {
	checker := versionedChecker("sshhostkeys")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "sshhostkeys version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := sshhostkeyDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["sshhostkeys"].([]interface{})
	return importSSHHostKeyList(sourceList, importFunc)
}

func importSSHHostKeyList(sourceList []interface{}, importFunc sshhostkeyDeserializationFunc) ([]*sshhostkey, error) {
	result := make([]*sshhostkey, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for sshhostkey %d, %T", i, value)
		}
		sshhostkey, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "sshhostkey %d", i)
		}
		result = append(result, sshhostkey)
	}
	return result, nil
}

type sshhostkeyDeserializationFunc func(map[string]interface{}) (*sshhostkey, error)

var sshhostkeyDeserializationFuncs = map[int]sshhostkeyDeserializationFunc{
	1: importSSHHostKeyV1,
}

func importSSHHostKeyV1(source map[string]interface{}) (*sshhostkey, error) {
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
		return nil, errors.Annotatef(err, "sshhostkey v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	dnsserversInterface := valid["dnsservers"].([]interface{})
	dnsservers := make([]string, len(dnsserversInterface))
	for i, d := range dnsserversInterface {
		dnsservers[i] = d.(string)
	}
	dnssearchInterface := valid["dnssearchdomains"].([]interface{})
	dnssearch := make([]string, len(dnssearchInterface))
	for i, d := range dnssearchInterface {
		dnssearch[i] = d.(string)
	}
	return &sshhostkey{
		ProviderID_:       valid["provider-id"].(string),
		DeviceName_:       valid["devicename"].(string),
		MachineID_:        valid["machineid"].(string),
		SubnetCIDR_:       valid["subnetcidr"].(string),
		ConfigMethod_:     valid["configmethod"].(string),
		Value_:            valid["value"].(string),
		DNSServers_:       dnsservers,
		DNSSearchDomains_: dnssearch,
		GatewayAddress_:   valid["gatewayaddress"].(string),
	}, nil
}
