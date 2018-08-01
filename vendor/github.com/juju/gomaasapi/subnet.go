// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type subnet struct {
	// Add the controller in when we need to do things with the subnet.
	// controller Controller

	resourceURI string

	id    int
	name  string
	space string
	vlan  *vlan

	gateway string
	cidr    string

	dnsServers []string
}

// ID implements Subnet.
func (s *subnet) ID() int {
	return s.id
}

// Name implements Subnet.
func (s *subnet) Name() string {
	return s.name
}

// Space implements Subnet.
func (s *subnet) Space() string {
	return s.space
}

// VLAN implements Subnet.
func (s *subnet) VLAN() VLAN {
	if s.vlan == nil {
		return nil
	}
	return s.vlan
}

// Gateway implements Subnet.
func (s *subnet) Gateway() string {
	return s.gateway
}

// CIDR implements Subnet.
func (s *subnet) CIDR() string {
	return s.cidr
}

// DNSServers implements Subnet.
func (s *subnet) DNSServers() []string {
	return s.dnsServers
}

func readSubnets(controllerVersion version.Number, source interface{}) ([]*subnet, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnet base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range subnetDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no subnet read func for version %s", controllerVersion)
	}
	readFunc := subnetDeserializationFuncs[deserialisationVersion]
	return readSubnetList(valid, readFunc)
}

// readSubnetList expects the values of the sourceList to be string maps.
func readSubnetList(sourceList []interface{}, readFunc subnetDeserializationFunc) ([]*subnet, error) {
	result := make([]*subnet, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for subnet %d, %T", i, value)
		}
		subnet, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "subnet %d", i)
		}
		result = append(result, subnet)
	}
	return result, nil
}

type subnetDeserializationFunc func(map[string]interface{}) (*subnet, error)

var subnetDeserializationFuncs = map[version.Number]subnetDeserializationFunc{
	twoDotOh: subnet_2_0,
}

func subnet_2_0(source map[string]interface{}) (*subnet, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),
		"id":           schema.ForceInt(),
		"name":         schema.String(),
		"space":        schema.String(),
		"gateway_ip":   schema.OneOf(schema.Nil(""), schema.String()),
		"cidr":         schema.String(),
		"vlan":         schema.StringMap(schema.Any()),
		"dns_servers":  schema.OneOf(schema.Nil(""), schema.List(schema.String())),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnet 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	vlan, err := vlan_2_0(valid["vlan"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Since the gateway_ip is optional, we use the two part cast assignment. If
	// the cast fails, then we get the default value we care about, which is the
	// empty string.
	gateway, _ := valid["gateway_ip"].(string)

	result := &subnet{
		resourceURI: valid["resource_uri"].(string),
		id:          valid["id"].(int),
		name:        valid["name"].(string),
		space:       valid["space"].(string),
		vlan:        vlan,
		gateway:     gateway,
		cidr:        valid["cidr"].(string),
		dnsServers:  convertToStringSlice(valid["dns_servers"]),
	}
	return result, nil
}
