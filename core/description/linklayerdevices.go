// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type linklayerdevices struct {
	Version           int                `yaml:"version"`
	LinkLayerDevices_ []*linklayerdevice `yaml:"linklayerdevices"`
}

type linklayerdevice struct {
	ProviderID_       string   `yaml:"provider-id,omitempty"`
	DeviceName_       string   `yaml:"devicename"`
	MachineID_        string   `yaml:"machineid"`
	SubnetCIDR_       string   `yaml:"subnetcidr"`
	ConfigMethod_     string   `yaml:"configmethod"`
	Value_            string   `yaml:"value"`
	DNSServers_       []string `yaml:"dnsservers"`
	DNSSearchDomains_ []string `yaml:"dnssearchdomains"`
	GatewayAddress_   string   `yaml:"gatewaydevice"`
}

// ProviderID implements LinkLayerDevice.
func (i *linklayerdevice) ProviderID() string {
	return i.ProviderID_
}

// DeviceName implements LinkLayerDevice.
func (i *linklayerdevice) DeviceName() string {
	return i.DeviceName_
}

// MachineID implements LinkLayerDevice.
func (i *linklayerdevice) MachineID() string {
	return i.MachineID_
}

// SubnetCIDR implements LinkLayerDevice.
func (i *linklayerdevice) SubnetCIDR() string {
	return i.SubnetCIDR_
}

// ConfigMethod implements LinkLayerDevice.
func (i *linklayerdevice) ConfigMethod() string {
	return i.ConfigMethod_
}

// Value implements LinkLayerDevice.
func (i *linklayerdevice) Value() string {
	return i.Value_
}

// DNSServers implements LinkLayerDevice.
func (i *linklayerdevice) DNSServers() []string {
	return i.DNSServers_
}

// DNSSearchDomains implements LinkLayerDevice.
func (i *linklayerdevice) DNSSearchDomains() []string {
	return i.DNSSearchDomains_
}

// GatewayAddress implements LinkLayerDevice.
func (i *linklayerdevice) GatewayAddress() string {
	return i.GatewayAddress_
}

// LinkLayerDeviceArgs is an argument struct used to create a
// new internal linklayerdevice type that supports the LinkLayerDevice interface.
type LinkLayerDeviceArgs struct {
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

func newLinkLayerDevice(args LinkLayerDeviceArgs) *linklayerdevice {
	return &linklayerdevice{
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

func importLinkLayerDevices(source map[string]interface{}) ([]*linklayerdevice, error) {
	checker := versionedChecker("linklayerdevices")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "linklayerdevices version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := linklayerdeviceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["linklayerdevices"].([]interface{})
	return importLinkLayerDeviceList(sourceList, importFunc)
}

func importLinkLayerDeviceList(sourceList []interface{}, importFunc linklayerdeviceDeserializationFunc) ([]*linklayerdevice, error) {
	result := make([]*linklayerdevice, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for linklayerdevice %d, %T", i, value)
		}
		linklayerdevice, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "linklayerdevice %d", i)
		}
		result = append(result, linklayerdevice)
	}
	return result, nil
}

type linklayerdeviceDeserializationFunc func(map[string]interface{}) (*linklayerdevice, error)

var linklayerdeviceDeserializationFuncs = map[int]linklayerdeviceDeserializationFunc{
	1: importLinkLayerDeviceV1,
}

func importLinkLayerDeviceV1(source map[string]interface{}) (*linklayerdevice, error) {
	fields := schema.Fields{
		"provider-id":      schema.String(),
		"devicename":       schema.String(),
		"machineid":        schema.String(),
		"subnetcidr":       schema.String(),
		"configmethod":     schema.String(),
		"value":            schema.String(),
		"dnsservers":       schema.List(schema.String()),
		"dnssearchdomains": schema.List(schema.String()),
		"gatewaydevice":    schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "linklayerdevice v1 schema check failed")
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
	return &linklayerdevice{
		ProviderID_:       valid["provider-id"].(string),
		DeviceName_:       valid["devicename"].(string),
		MachineID_:        valid["machineid"].(string),
		SubnetCIDR_:       valid["subnetcidr"].(string),
		ConfigMethod_:     valid["configmethod"].(string),
		Value_:            valid["value"].(string),
		DNSServers_:       dnsservers,
		DNSSearchDomains_: dnssearch,
		GatewayAddress_:   valid["gatewaydevice"].(string),
	}, nil
}
