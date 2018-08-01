// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// CloudContainer represents the state of a CAAS container, eg pod.
type CloudContainer interface {
	ProviderId() string
	Address() Address
	Ports() []string
}

type cloudContainer struct {
	Version int `yaml:"version"`

	ProviderId_ string   `yaml:"provider-id,omitempty"`
	Address_    *address `yaml:"address,omitempty"`
	Ports_      []string `yaml:"ports,omitempty"`
}

// ProviderId implements CloudContainer.
func (c *cloudContainer) ProviderId() string {
	return c.ProviderId_
}

// Address implements CloudContainer.
func (c *cloudContainer) Address() Address {
	return c.Address_
}

// Ports implements CloudContainer.
func (c *cloudContainer) Ports() []string {
	return c.Ports_
}

// CloudContainerArgs is an argument struct used to create a
// new internal cloudContainer type that supports the CloudContainer interface.
type CloudContainerArgs struct {
	ProviderId string
	Address    AddressArgs
	Ports      []string
}

func newCloudContainer(args *CloudContainerArgs) *cloudContainer {
	if args == nil {
		return nil
	}
	cloudcontainer := &cloudContainer{
		Version:     1,
		ProviderId_: args.ProviderId,
		Address_:    newAddress(args.Address),
		Ports_:      args.Ports,
	}
	return cloudcontainer
}

func importCloudContainer(source map[string]interface{}) (*cloudContainer, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "cloudContainer version schema check failed")
	}

	importFunc, ok := cloudContainerDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	return importFunc(source)
}

type cloudContainerDeserializationFunc func(map[string]interface{}) (*cloudContainer, error)

var cloudContainerDeserializationFuncs = map[int]cloudContainerDeserializationFunc{
	1: importCloudContainerV1,
}

func importCloudContainerV1(source map[string]interface{}) (*cloudContainer, error) {
	fields := schema.Fields{
		"provider-id": schema.String(),
		"address":     schema.StringMap(schema.Any()),
		"ports":       schema.List(schema.String()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": schema.Omit,
		"address":     schema.Omit,
		"ports":       schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudContainer v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	cloudContainer := &cloudContainer{
		Version:     1,
		ProviderId_: valid["provider-id"].(string),
		Ports_:      convertToStringSlice(valid["ports"]),
	}

	if address, ok := valid["address"]; ok {
		containerAddresses, err := importAddress(address.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudContainer.Address_ = containerAddresses
	}

	return cloudContainer, nil
}
