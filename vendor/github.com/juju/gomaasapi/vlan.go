// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type vlan struct {
	// Add the controller in when we need to do things with the vlan.
	// controller Controller

	resourceURI string

	id     int
	name   string
	fabric string

	vid  int
	mtu  int
	dhcp bool

	primaryRack   string
	secondaryRack string
}

// ID implements VLAN.
func (v *vlan) ID() int {
	return v.id
}

// Name implements VLAN.
func (v *vlan) Name() string {
	return v.name
}

// Fabric implements VLAN.
func (v *vlan) Fabric() string {
	return v.fabric
}

// VID implements VLAN.
func (v *vlan) VID() int {
	return v.vid
}

// MTU implements VLAN.
func (v *vlan) MTU() int {
	return v.mtu
}

// DHCP implements VLAN.
func (v *vlan) DHCP() bool {
	return v.dhcp
}

// PrimaryRack implements VLAN.
func (v *vlan) PrimaryRack() string {
	return v.primaryRack
}

// SecondaryRack implements VLAN.
func (v *vlan) SecondaryRack() string {
	return v.secondaryRack
}

func readVLANs(controllerVersion version.Number, source interface{}) ([]*vlan, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "vlan base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range vlanDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no vlan read func for version %s", controllerVersion)
	}
	readFunc := vlanDeserializationFuncs[deserialisationVersion]
	return readVLANList(valid, readFunc)
}

func readVLANList(sourceList []interface{}, readFunc vlanDeserializationFunc) ([]*vlan, error) {
	result := make([]*vlan, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for vlan %d, %T", i, value)
		}
		vlan, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "vlan %d", i)
		}
		result = append(result, vlan)
	}
	return result, nil
}

type vlanDeserializationFunc func(map[string]interface{}) (*vlan, error)

var vlanDeserializationFuncs = map[version.Number]vlanDeserializationFunc{
	twoDotOh: vlan_2_0,
}

func vlan_2_0(source map[string]interface{}) (*vlan, error) {
	fields := schema.Fields{
		"id":           schema.ForceInt(),
		"resource_uri": schema.String(),
		"name":         schema.OneOf(schema.Nil(""), schema.String()),
		"fabric":       schema.String(),
		"vid":          schema.ForceInt(),
		"mtu":          schema.ForceInt(),
		"dhcp_on":      schema.Bool(),
		// racks are not always set.
		"primary_rack":   schema.OneOf(schema.Nil(""), schema.String()),
		"secondary_rack": schema.OneOf(schema.Nil(""), schema.String()),
	}
	checker := schema.FieldMap(fields, nil)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "vlan 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	// Since the primary and secondary racks are optional, we use the two
	// part cast assignment. If the case fails, then we get the default value
	// we care about, which is the empty string.
	primary_rack, _ := valid["primary_rack"].(string)
	secondary_rack, _ := valid["secondary_rack"].(string)
	name, _ := valid["name"].(string)

	result := &vlan{
		resourceURI:   valid["resource_uri"].(string),
		id:            valid["id"].(int),
		name:          name,
		fabric:        valid["fabric"].(string),
		vid:           valid["vid"].(int),
		mtu:           valid["mtu"].(int),
		dhcp:          valid["dhcp_on"].(bool),
		primaryRack:   primary_rack,
		secondaryRack: secondary_rack,
	}
	return result, nil
}
