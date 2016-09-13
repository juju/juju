// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type subnets struct {
	Version  int       `yaml:"version"`
	Subnets_ []*subnet `yaml:"subnets"`
}

type subnet struct {
	ProviderId_ string `yaml:"provider-id,omitempty"`
	CIDR_       string `yaml:"cidr"`
	VLANTag_    int    `yaml:"vlan-tag"`

	AvailabilityZone_ string `yaml:"availability-zone"`
	SpaceName_        string `yaml:"space-name"`

	// These will be deprecated once the address allocation strategy for
	// EC2 is changed. They are unused already on MAAS.
	AllocatableIPHigh_ string `yaml:"allocatable-ip-high,omitempty"`
	AllocatableIPLow_  string `yaml:"allocatable-ip-low,omitempty"`
}

// SubnetArgs is an argument struct used to create a
// new internal subnet type that supports the Subnet interface.
type SubnetArgs struct {
	ProviderId       string
	CIDR             string
	VLANTag          int
	AvailabilityZone string
	SpaceName        string

	// These will be deprecated once the address allocation strategy for
	// EC2 is changed. They are unused already on MAAS.
	AllocatableIPHigh string
	AllocatableIPLow  string
}

func newSubnet(args SubnetArgs) *subnet {
	return &subnet{
		ProviderId_:        string(args.ProviderId),
		SpaceName_:         args.SpaceName,
		CIDR_:              args.CIDR,
		VLANTag_:           args.VLANTag,
		AvailabilityZone_:  args.AvailabilityZone,
		AllocatableIPHigh_: args.AllocatableIPHigh,
		AllocatableIPLow_:  args.AllocatableIPLow,
	}
}

// ProviderId implements Subnet.
func (s *subnet) ProviderId() string {
	return s.ProviderId_
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

// AvailabilityZone implements Subnet.
func (s *subnet) AvailabilityZone() string {
	return s.AvailabilityZone_
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
	importFunc, ok := subnetDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["subnets"].([]interface{})
	return importSubnetList(sourceList, importFunc)
}

func importSubnetList(sourceList []interface{}, importFunc subnetDeserializationFunc) ([]*subnet, error) {
	result := make([]*subnet, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for subnet %d, %T", i, value)
		}
		subnet, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "subnet %d", i)
		}
		result = append(result, subnet)
	}
	return result, nil
}

type subnetDeserializationFunc func(map[string]interface{}) (*subnet, error)

var subnetDeserializationFuncs = map[int]subnetDeserializationFunc{
	1: importSubnetV1,
}

func importSubnetV1(source map[string]interface{}) (*subnet, error) {
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
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnet v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	return &subnet{
		CIDR_:              valid["cidr"].(string),
		ProviderId_:        valid["provider-id"].(string),
		VLANTag_:           int(valid["vlan-tag"].(int64)),
		SpaceName_:         valid["space-name"].(string),
		AvailabilityZone_:  valid["availability-zone"].(string),
		AllocatableIPHigh_: valid["allocatable-ip-high"].(string),
		AllocatableIPLow_:  valid["allocatable-ip-low"].(string),
	}, nil
}
