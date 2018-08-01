// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// Subnet represents a network subnet.
type Subnet interface {
	ProviderId() string
	ProviderNetworkId() string
	ProviderSpaceId() string
	CIDR() string
	VLANTag() int
	AvailabilityZones() []string
	SpaceName() string
	FanLocalUnderlay() string
	FanOverlay() string
	AllocatableIPHigh() string
	AllocatableIPLow() string
}

type subnets struct {
	Version  int       `yaml:"version"`
	Subnets_ []*subnet `yaml:"subnets"`
}

type subnet struct {
	ProviderId_        string `yaml:"provider-id,omitempty"`
	ProviderNetworkId_ string `yaml:"provider-network-id,omitempty"`
	ProviderSpaceId_   string `yaml:"provider-space-id,omitempty"`
	CIDR_              string `yaml:"cidr"`
	VLANTag_           int    `yaml:"vlan-tag"`

	AvailabilityZones_ []string `yaml:"availability-zones"`
	SpaceName_         string   `yaml:"space-name"`

	FanLocalUnderlay_ string `yaml:"fan-local-underlay,omitempty"`
	FanOverlay_       string `yaml:"fan-overlay,omitempty"`

	// These will be deprecated once the address allocation strategy for
	// EC2 is changed. They are unused already on MAAS.
	AllocatableIPHigh_ string `yaml:"allocatable-ip-high,omitempty"`
	AllocatableIPLow_  string `yaml:"allocatable-ip-low,omitempty"`
}

// SubnetArgs is an argument struct used to create a
// new internal subnet type that supports the Subnet interface.
type SubnetArgs struct {
	ProviderId        string
	ProviderNetworkId string
	ProviderSpaceId   string
	CIDR              string
	VLANTag           int
	AvailabilityZones []string
	SpaceName         string
	FanLocalUnderlay  string
	FanOverlay        string

	// These will be deprecated once the address allocation strategy for
	// EC2 is changed. They are unused already on MAAS.
	AllocatableIPHigh string
	AllocatableIPLow  string
}

func newSubnet(args SubnetArgs) *subnet {
	return &subnet{
		ProviderId_:        args.ProviderId,
		ProviderNetworkId_: args.ProviderNetworkId,
		ProviderSpaceId_:   args.ProviderSpaceId,
		SpaceName_:         args.SpaceName,
		CIDR_:              args.CIDR,
		VLANTag_:           args.VLANTag,
		AvailabilityZones_: args.AvailabilityZones,
		FanLocalUnderlay_:  args.FanLocalUnderlay,
		FanOverlay_:        args.FanOverlay,
		AllocatableIPHigh_: args.AllocatableIPHigh,
		AllocatableIPLow_:  args.AllocatableIPLow,
	}
}

// ProviderId implements Subnet.
func (s *subnet) ProviderId() string {
	return s.ProviderId_
}

// ProviderNetworkId implements Subnet.
func (s *subnet) ProviderNetworkId() string {
	return s.ProviderNetworkId_
}

// ProviderSpaceId implements Subnet.
func (s *subnet) ProviderSpaceId() string {
	return s.ProviderSpaceId_
}

// SpaceName implements Subnet.
func (s *subnet) SpaceName() string {
	return s.SpaceName_
}

// CIDR implements Subnet.
func (s *subnet) CIDR() string {
	return s.CIDR_
}

// VLANTag implements Subnet.
func (s *subnet) VLANTag() int {
	return s.VLANTag_
}

// AvailabilityZones implements Subnet.
func (s *subnet) AvailabilityZones() []string {
	return s.AvailabilityZones_
}

// FanLocalUnderlay implements Subnet.
func (s *subnet) FanLocalUnderlay() string {
	return s.FanLocalUnderlay_
}

// FanOverlay implements Subnet.
func (s *subnet) FanOverlay() string {
	return s.FanOverlay_
}

// AllocatableIPHigh implements Subnet.
func (s *subnet) AllocatableIPHigh() string {
	return s.AllocatableIPHigh_
}

// AllocatableIPLow implements Subnet.
func (s *subnet) AllocatableIPLow() string {
	return s.AllocatableIPLow_
}

func importSubnets(source map[string]interface{}) ([]*subnet, error) {
	checker := versionedChecker("subnets")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnets version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	getFields, ok := subnetFieldsFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["subnets"].([]interface{})
	return importSubnetList(sourceList, schema.FieldMap(getFields()), version)
}

func importSubnetList(sourceList []interface{}, checker schema.Checker, version int) ([]*subnet, error) {
	result := make([]*subnet, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for subnet %d, %T", i, value)
		}
		coerced, err := checker.Coerce(source, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "subnet %d v%d schema check failed", i, version)
		}
		valid := coerced.(map[string]interface{})
		subnet, err := newSubnetFromValid(valid, version)
		if err != nil {
			return nil, errors.Annotatef(err, "subnet %d", i)
		}
		result = append(result, subnet)
	}
	return result, nil
}

var subnetFieldsFuncs = map[int]fieldsFunc{
	1: subnetV1Fields,
	2: subnetV2Fields,
	3: subnetV3Fields,
	4: subnetV4Fields,
}

func newSubnetFromValid(valid map[string]interface{}, version int) (*subnet, error) {
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := subnet{
		CIDR_:              valid["cidr"].(string),
		ProviderId_:        valid["provider-id"].(string),
		VLANTag_:           int(valid["vlan-tag"].(int64)),
		SpaceName_:         valid["space-name"].(string),
		AllocatableIPHigh_: valid["allocatable-ip-high"].(string),
		AllocatableIPLow_:  valid["allocatable-ip-low"].(string),
	}
	if version >= 2 {
		result.ProviderNetworkId_ = valid["provider-network-id"].(string)
	}
	if version >= 3 {
		result.ProviderSpaceId_ = valid["provider-space-id"].(string)
		result.AvailabilityZones_ = convertToStringSlice(valid["availability-zones"])
	} else {
		result.AvailabilityZones_ = []string{valid["availability-zone"].(string)}
	}
	if version >= 4 {
		result.FanLocalUnderlay_ = valid["fan-local-underlay"].(string)
		result.FanOverlay_ = valid["fan-overlay"].(string)
	}
	return &result, nil
}

func subnetV1Fields() (schema.Fields, schema.Defaults) {
	fields := schema.Fields{
		"cidr":                schema.String(),
		"provider-id":         schema.String(),
		"vlan-tag":            schema.Int(),
		"space-name":          schema.String(),
		"availability-zone":   schema.String(),
		"allocatable-ip-high": schema.String(),
		"allocatable-ip-low":  schema.String(),
	}
	defaults := schema.Defaults{
		"provider-id":         "",
		"allocatable-ip-high": "",
		"allocatable-ip-low":  "",
	}
	return fields, defaults
}

func subnetV2Fields() (schema.Fields, schema.Defaults) {
	fields, defaults := subnetV1Fields()
	fields["provider-network-id"] = schema.String()
	defaults["provider-network-id"] = ""
	return fields, defaults
}

func subnetV3Fields() (schema.Fields, schema.Defaults) {
	fields, defaults := subnetV2Fields()
	fields["provider-space-id"] = schema.String()
	fields["availability-zones"] = schema.List(schema.String())
	delete(fields, "availability-zone")
	defaults["provider-space-id"] = ""
	return fields, defaults
}

func subnetV4Fields() (schema.Fields, schema.Defaults) {
	fields, defaults := subnetV3Fields()
	fields["fan-local-underlay"] = schema.String()
	fields["fan-overlay"] = schema.String()
	defaults["fan-local-underlay"] = ""
	defaults["fan-overlay"] = ""
	return fields, defaults
}
