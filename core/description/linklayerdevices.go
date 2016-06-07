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
	Name_        string `yaml:"name"`
	MTU_         uint   `yaml:"mtu"`
	ProviderID_  string `yaml:"provider-id,omitempty"`
	MachineID_   string `yaml:"machineid"`
	Type_        string `yaml:"type"`
	MACAddress_  string `yaml:"macaddress"`
	IsAutoStart_ bool   `yaml:"isautostart"`
	IsUp_        bool   `yaml:"isup"`
	ParentName_  string `yaml:"parentname"`
}

// ProviderID implements LinkLayerDevice.
func (i *linklayerdevice) ProviderID() string {
	return i.ProviderID_
}

// MachineID implements LinkLayerDevice.
func (i *linklayerdevice) MachineID() string {
	return i.MachineID_
}

// Name implements LinkLayerDevice.
func (i *linklayerdevice) Name() string {
	return i.Name_
}

// MTU implements LinkLayerDevice.
func (i *linklayerdevice) MTU() uint {
	return i.MTU_
}

// Type implements LinkLayerDevice.
func (i *linklayerdevice) Type() string {
	return i.Type_
}

// MACAddress implements LinkLayerDevice.
func (i *linklayerdevice) MACAddress() string {
	return i.MACAddress_
}

// IsAutoStart implements LinkLayerDevice.
func (i *linklayerdevice) IsAutoStart() bool {
	return i.IsAutoStart_
}

// IsUp implements LinkLayerDevice.
func (i *linklayerdevice) IsUp() bool {
	return i.IsUp_
}

// ParentName implements LinkLayerDevice.
func (i *linklayerdevice) ParentName() string {
	return i.ParentName_
}

// LinkLayerDeviceArgs is an argument struct used to create a
// new internal linklayerdevice type that supports the LinkLayerDevice interface.
type LinkLayerDeviceArgs struct {
	Name        string
	MTU         uint
	ProviderID  string
	MachineID   string
	Type        string
	MACAddress  string
	IsAutoStart bool
	IsUp        bool
	ParentName  string
}

func newLinkLayerDevice(args LinkLayerDeviceArgs) *linklayerdevice {
	return &linklayerdevice{
		ProviderID_:  args.ProviderID,
		MachineID_:   args.MachineID,
		Name_:        args.Name,
		MTU_:         args.MTU,
		Type_:        args.Type,
		MACAddress_:  args.MACAddress,
		IsAutoStart_: args.IsAutoStart,
		IsUp_:        args.IsUp,
		ParentName_:  args.ParentName,
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
		"provider-id": schema.String(),
		"machineid":   schema.String(),
		"name":        schema.String(),
		"mtu":         schema.Int(),
		"type":        schema.String(),
		"macaddress":  schema.String(),
		"isautostart": schema.Bool(),
		"isup":        schema.Bool(),
		"parentname":  schema.String(),
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
	return &linklayerdevice{
		ProviderID_:  valid["provider-id"].(string),
		MachineID_:   valid["machineid"].(string),
		Name_:        valid["name"].(string),
		MTU_:         valid["mtu"].(uint),
		Type_:        valid["type"].(string),
		MACAddress_:  valid["macaddress"].(string),
		IsAutoStart_: valid["isautostart"].(bool),
		IsUp_:        valid["isup"].(bool),
		ParentName_:  valid["parentname"].(string),
	}, nil
}
