// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// BlockDevice represents a block device on a machine.
type BlockDevice interface {
	Name() string
	Links() []string
	Label() string
	UUID() string
	HardwareID() string
	BusAddress() string
	Size() uint64
	FilesystemType() string
	InUse() bool
	MountPoint() string
}

type blockdevices struct {
	Version       int            `yaml:"version"`
	BlockDevices_ []*blockdevice `yaml:"block-devices"`
}

func (d *blockdevices) add(args BlockDeviceArgs) *blockdevice {
	dev := newBlockDevice(args)
	d.BlockDevices_ = append(d.BlockDevices_, dev)
	return dev
}

type blockdevice struct {
	Name_           string   `yaml:"name"`
	Links_          []string `yaml:"links,omitempty"`
	Label_          string   `yaml:"label,omitempty"`
	UUID_           string   `yaml:"uuid,omitempty"`
	HardwareID_     string   `yaml:"hardware-id,omitempty"`
	BusAddress_     string   `yaml:"bus-address,omitempty"`
	Size_           uint64   `yaml:"size"`
	FilesystemType_ string   `yaml:"fs-type,omitempty"`
	InUse_          bool     `yaml:"in-use"`
	MountPoint_     string   `yaml:"mount-point,omitempty"`
}

// BlockDeviceArgs is an argument struct used to add a block device to a Machine.
type BlockDeviceArgs struct {
	Name           string
	Links          []string
	Label          string
	UUID           string
	HardwareID     string
	BusAddress     string
	Size           uint64
	FilesystemType string
	InUse          bool
	MountPoint     string
}

func newBlockDevice(args BlockDeviceArgs) *blockdevice {
	bd := &blockdevice{
		Name_:           args.Name,
		Links_:          make([]string, len(args.Links)),
		Label_:          args.Label,
		UUID_:           args.UUID,
		HardwareID_:     args.HardwareID,
		BusAddress_:     args.BusAddress,
		Size_:           args.Size,
		FilesystemType_: args.FilesystemType,
		InUse_:          args.InUse,
		MountPoint_:     args.MountPoint,
	}
	copy(bd.Links_, args.Links)
	return bd
}

// Name implements BlockDevice.
func (b *blockdevice) Name() string {
	return b.Name_
}

// Links implements BlockDevice.
func (b *blockdevice) Links() []string {
	return b.Links_
}

// Label implements BlockDevice.
func (b *blockdevice) Label() string {
	return b.Label_
}

// UUID implements BlockDevice.
func (b *blockdevice) UUID() string {
	return b.UUID_
}

// HardwareID implements BlockDevice.
func (b *blockdevice) HardwareID() string {
	return b.HardwareID_
}

// BusAddress implements BlockDevice.
func (b *blockdevice) BusAddress() string {
	return b.BusAddress_
}

// Size implements BlockDevice.
func (b *blockdevice) Size() uint64 {
	return b.Size_
}

// FilesystemType implements BlockDevice.
func (b *blockdevice) FilesystemType() string {
	return b.FilesystemType_
}

// InUse implements BlockDevice.
func (b *blockdevice) InUse() bool {
	return b.InUse_
}

// MountPoint implements BlockDevice.
func (b *blockdevice) MountPoint() string {
	return b.MountPoint_
}

func importBlockDevices(source interface{}) ([]*blockdevice, error) {
	checker := versionedChecker("block-devices")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "block devices version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := blockdeviceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["block-devices"].([]interface{})
	return importBlockDeviceList(sourceList, importFunc)
}

func importBlockDeviceList(sourceList []interface{}, importFunc blockdeviceDeserializationFunc) ([]*blockdevice, error) {
	result := make([]*blockdevice, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for block device %d, %T", i, value)
		}
		device, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "block device %d", i)
		}
		result = append(result, device)
	}
	return result, nil
}

type blockdeviceDeserializationFunc func(map[string]interface{}) (*blockdevice, error)

var blockdeviceDeserializationFuncs = map[int]blockdeviceDeserializationFunc{
	1: importBlockDeviceV1,
}

func importBlockDeviceV1(source map[string]interface{}) (*blockdevice, error) {
	fields := schema.Fields{
		"name":        schema.String(),
		"links":       schema.List(schema.String()),
		"label":       schema.String(),
		"uuid":        schema.String(),
		"hardware-id": schema.String(),
		"bus-address": schema.String(),
		"size":        schema.ForceUint(),
		"fs-type":     schema.String(),
		"in-use":      schema.Bool(),
		"mount-point": schema.String(),
	}

	defaults := schema.Defaults{
		"links":       schema.Omit,
		"label":       "",
		"uuid":        "",
		"hardware-id": "",
		"bus-address": "",
		"fs-type":     "",
		"mount-point": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "block device v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &blockdevice{
		Name_:           valid["name"].(string),
		Links_:          convertToStringSlice(valid["links"]),
		Label_:          valid["label"].(string),
		UUID_:           valid["uuid"].(string),
		HardwareID_:     valid["hardware-id"].(string),
		BusAddress_:     valid["bus-address"].(string),
		Size_:           valid["size"].(uint64),
		FilesystemType_: valid["fs-type"].(string),
		InUse_:          valid["in-use"].(bool),
		MountPoint_:     valid["mount-point"].(string),
	}

	return result, nil
}
