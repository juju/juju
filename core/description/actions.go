// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type actions struct {
	Version      int          `yaml:"version"`
	Actions_ []*action `yaml:"actions"`
}

type action struct {
	ProviderID_       string   `yaml:"provider-id,omitempty"`
	DeviceName_       string   `yaml:"devicename"`
	MachineID_        string   `yaml:"machineid"`
	SubnetCIDR_       string   `yaml:"subnetcidr"`
	ConfigMethod_     string   `yaml:"configmethod"`
	Value_            string   `yaml:"value"`
	DNSServers_       []string `yaml:"dnsservers"`
	DNSSearchDomains_ []string `yaml:"dnssearchdomains"`
	GatewayAddress_   string   `yaml:"gatewayaction"`
}

// ProviderID implements Action.
func (i *action) ProviderID() string {
	return i.ProviderID_
}

// DeviceName implements Action.
func (i *action) DeviceName() string {
	return i.DeviceName_
}

// MachineID implements Action.
func (i *action) MachineID() string {
	return i.MachineID_
}

// SubnetCIDR implements Action.
func (i *action) SubnetCIDR() string {
	return i.SubnetCIDR_
}

// ConfigMethod implements Action.
func (i *action) ConfigMethod() string {
	return i.ConfigMethod_
}

// Value implements Action.
func (i *action) Value() string {
	return i.Value_
}

// DNSServers implements Action.
func (i *action) DNSServers() []string {
	return i.DNSServers_
}

// DNSSearchDomains implements Action.
func (i *action) DNSSearchDomains() []string {
	return i.DNSSearchDomains_
}

// GatewayAddress implements Action.
func (i *action) GatewayAddress() string {
	return i.GatewayAddress_
}

// ActionArgs is an argument struct used to create a
// new internal action type that supports the Action interface.
type ActionArgs struct {
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

func newAction(args ActionArgs) *action {
	return &action{
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

func importActions(source map[string]interface{}) ([]*action, error) {
	checker := versionedChecker("actions")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "actions version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := actionDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["actions"].([]interface{})
	return importActionList(sourceList, importFunc)
}

func importActionList(sourceList []interface{}, importFunc actionDeserializationFunc) ([]*action, error) {
	result := make([]*action, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for action %d, %T", i, value)
		}
		action, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "action %d", i)
		}
		result = append(result, action)
	}
	return result, nil
}

type actionDeserializationFunc func(map[string]interface{}) (*action, error)

var actionDeserializationFuncs = map[int]actionDeserializationFunc{
	1: importActionV1,
}

func importActionV1(source map[string]interface{}) (*action, error) {
	fields := schema.Fields{
		"provider-id":      schema.String(),
		"devicename":       schema.String(),
		"machineid":        schema.String(),
		"subnetcidr":       schema.String(),
		"configmethod":     schema.String(),
		"value":            schema.String(),
		"dnsservers":       schema.List(schema.String()),
		"dnssearchdomains": schema.List(schema.String()),
		"gatewayaction":   schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "action v1 schema check failed")
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
	return &action{
		ProviderID_:       valid["provider-id"].(string),
		DeviceName_:       valid["devicename"].(string),
		MachineID_:        valid["machineid"].(string),
		SubnetCIDR_:       valid["subnetcidr"].(string),
		ConfigMethod_:     valid["configmethod"].(string),
		Value_:            valid["value"].(string),
		DNSServers_:       dnsservers,
		DNSSearchDomains_: dnssearch,
		GatewayAddress_:   valid["gatewayaction"].(string),
	}, nil
}
