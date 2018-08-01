// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// RemoteSpace represents a network space that endpoints of remote
// applications might be connected to.
type RemoteSpace interface {
	CloudType() string
	Name() string
	ProviderId() string
	ProviderAttributes() map[string]interface{}

	Subnets() []Subnet
	AddSubnet(SubnetArgs) Subnet
}

type remoteSpaces struct {
	Version int            `yaml:"version"`
	Spaces  []*remoteSpace `yaml:"spaces"`
}

type remoteSpace struct {
	CloudType_          string                 `yaml:"cloud-type"`
	Name_               string                 `yaml:"name"`
	ProviderId_         string                 `yaml:"provider-id"`
	ProviderAttributes_ map[string]interface{} `yaml:"provider-attributes"`
	Subnets_            subnets                `yaml:"subnets,omitempty"`
}

// RemoteSpaceArgs is an argument struct used to add a remote space to
// a remote application.
type RemoteSpaceArgs struct {
	CloudType          string
	Name               string
	ProviderId         string
	ProviderAttributes map[string]interface{}
}

func newRemoteSpace(args RemoteSpaceArgs) *remoteSpace {
	s := &remoteSpace{
		CloudType_:          args.CloudType,
		Name_:               args.Name,
		ProviderId_:         args.ProviderId,
		ProviderAttributes_: args.ProviderAttributes,
	}
	s.setSubnets(nil)
	return s
}

// CloudType implements RemoteSpace.
func (s *remoteSpace) CloudType() string {
	return s.CloudType_
}

// Name implements RemoteSpace.
func (s *remoteSpace) Name() string {
	return s.Name_
}

// ProviderId implements RemoteSpace.
func (s *remoteSpace) ProviderId() string {
	return s.ProviderId_
}

// ProviderAttributes implements RemoteSpace.
func (s *remoteSpace) ProviderAttributes() map[string]interface{} {
	return s.ProviderAttributes_
}

// Subnets implements RemoteSpace.
func (s *remoteSpace) Subnets() []Subnet {
	result := make([]Subnet, len(s.Subnets_.Subnets_))
	for i, subnet := range s.Subnets_.Subnets_ {
		result[i] = subnet
	}
	return result
}

// AddSubnet implements RemoteSpace.
func (s *remoteSpace) AddSubnet(args SubnetArgs) Subnet {
	sn := newSubnet(args)
	s.Subnets_.Subnets_ = append(s.Subnets_.Subnets_, sn)
	return sn
}

func (s *remoteSpace) setSubnets(subnetList []*subnet) {
	s.Subnets_ = subnets{
		Version:  3,
		Subnets_: subnetList,
	}
}

func importRemoteSpaces(source map[string]interface{}) ([]*remoteSpace, error) {
	checker := versionedChecker("spaces")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "remote spaces version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	getFields, ok := remoteSpaceFieldsFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["spaces"].([]interface{})
	return importRemoteSpaceList(sourceList, schema.FieldMap(getFields()), version)
}

func importRemoteSpaceList(sourceList []interface{}, checker schema.Checker, version int) ([]*remoteSpace, error) {
	var result []*remoteSpace
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for remote space %d, %T", i, value)
		}
		coerced, err := checker.Coerce(source, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "remote space %d v%d schema check failed", i, version)
		}
		valid := coerced.(map[string]interface{})
		space, err := newRemoteSpaceFromValid(valid, version)
		if err != nil {
			return nil, errors.Annotatef(err, "remote space %d", i)
		}
		result = append(result, space)
	}
	return result, nil
}

func newRemoteSpaceFromValid(valid map[string]interface{}, version int) (*remoteSpace, error) {
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := remoteSpace{
		CloudType_:          valid["cloud-type"].(string),
		Name_:               valid["name"].(string),
		ProviderId_:         valid["provider-id"].(string),
		ProviderAttributes_: valid["provider-attributes"].(map[string]interface{}),
	}
	if rawSubnets, ok := valid["subnets"]; ok {
		subnetsMap := rawSubnets.(map[string]interface{})
		subnets, err := importSubnets(subnetsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.setSubnets(subnets)
	}

	return &result, nil
}

var remoteSpaceFieldsFuncs = map[int]fieldsFunc{
	1: remoteSpaceV1Fields,
}

func remoteSpaceV1Fields() (schema.Fields, schema.Defaults) {
	fields := schema.Fields{
		"cloud-type":          schema.String(),
		"name":                schema.String(),
		"provider-id":         schema.String(),
		"provider-attributes": schema.StringMap(schema.Any()),
		"subnets":             schema.StringMap(schema.Any()),
	}
	defaults := schema.Defaults{
		"subnets": schema.Omit,
	}
	return fields, defaults
}
