// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

type volumes struct {
	Version  int       `yaml:"version"`
	Volumes_ []*volume `yaml:"volumes"`
}

type volume struct {
	ID_          string `yaml:"id"`
	Binding_     string `yaml:"binding,omitempty"`
	StorageID_   string `yaml:"storage-id,omitempty"`
	Provisioned_ bool   `yaml:"provisioned"`
	Size_        uint64 `yaml:"size"`
	Pool_        string `yaml:"pool,omitempty"`
	HardwareID_  string `yaml:"hardware-id,omitempty"`
	VolumeID_    string `yaml:"volume-id,omitempty"`
	Persistent_  bool   `yaml:"persistent"`

	Status_        *status `yaml:"status"`
	StatusHistory_ `yaml:"status-history"`

	Attachments_ volumeAttachments `yaml:"attachments"`
}

type volumeAttachments struct {
	Version      int                 `yaml:"version"`
	Attachments_ []*volumeAttachment `yaml:"attachments"`
}

type volumeAttachment struct {
	MachineID_   string `yaml:"machine-id"`
	Provisioned_ bool   `yaml:"provisioned"`
	ReadOnly_    bool   `yaml:"read-only"`
	DeviceName_  string `yaml:"device-name"`
	DeviceLink_  string `yaml:"device-link"`
	BusAddress_  string `yaml:"bus-address"`
}

// VolumeArgs is an argument struct used to add a volume to the Model.
type VolumeArgs struct {
	Tag         names.VolumeTag
	Storage     names.StorageTag
	Binding     names.Tag
	Provisioned bool
	Size        uint64
	Pool        string
	HardwareID  string
	VolumeID    string
	Persistent  bool
}

func newVolume(args VolumeArgs) *volume {
	v := &volume{
		ID_:            args.Tag.Id(),
		StorageID_:     args.Storage.Id(),
		Provisioned_:   args.Provisioned,
		Size_:          args.Size,
		Pool_:          args.Pool,
		HardwareID_:    args.HardwareID,
		VolumeID_:      args.VolumeID,
		Persistent_:    args.Persistent,
		StatusHistory_: newStatusHistory(),
	}
	if args.Binding != nil {
		v.Binding_ = args.Binding.String()
	}
	v.setAttachments(nil)
	return v
}

// Tag implements Volume.
func (v *volume) Tag() names.VolumeTag {
	return names.NewVolumeTag(v.ID_)
}

// Storage implements Volume.
func (v *volume) Storage() names.StorageTag {
	if v.StorageID_ == "" {
		return names.StorageTag{}
	}
	return names.NewStorageTag(v.StorageID_)
}

// Binding implements Volume.
func (v *volume) Binding() (names.Tag, error) {
	if v.Binding_ == "" {
		return nil, nil
	}
	tag, err := names.ParseTag(v.Binding_)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tag, nil
}

// Provisioned implements Volume.
func (v *volume) Provisioned() bool {
	return v.Provisioned_
}

// Size implements Volume.
func (v *volume) Size() uint64 {
	return v.Size_
}

// Pool implements Volume.
func (v *volume) Pool() string {
	return v.Pool_
}

// HardwareID implements Volume.
func (v *volume) HardwareID() string {
	return v.HardwareID_
}

// VolumeID implements Volume.
func (v *volume) VolumeID() string {
	return v.VolumeID_
}

// Persistent implements Volume.
func (v *volume) Persistent() bool {
	return v.Persistent_
}

// Status implements Volume.
func (v *volume) Status() Status {
	// To avoid typed nils check nil here.
	if v.Status_ == nil {
		return nil
	}
	return v.Status_
}

// SetStatus implements Volume.
func (v *volume) SetStatus(args StatusArgs) {
	v.Status_ = newStatus(args)
}

func (v *volume) setAttachments(attachments []*volumeAttachment) {
	v.Attachments_ = volumeAttachments{
		Version:      1,
		Attachments_: attachments,
	}
}

// Attachments implements Volume.
func (v *volume) Attachments() []VolumeAttachment {
	var result []VolumeAttachment
	for _, attachment := range v.Attachments_.Attachments_ {
		result = append(result, attachment)
	}
	return result
}

// AddAttachment implements Volume.
func (v *volume) AddAttachment(args VolumeAttachmentArgs) VolumeAttachment {
	a := newVolumeAttachment(args)
	v.Attachments_.Attachments_ = append(v.Attachments_.Attachments_, a)
	return a
}

// Validate implements Volume.
func (v *volume) Validate() error {
	if v.ID_ == "" {
		return errors.NotValidf("volume missing id")
	}
	if v.Size_ == 0 {
		return errors.NotValidf("volume %q missing size", v.ID_)
	}
	if v.Status_ == nil {
		return errors.NotValidf("volume %q missing status", v.ID_)
	}
	if _, err := v.Binding(); err != nil {
		return errors.Wrap(err, errors.NotValidf("volume %q binding", v.ID_))
	}
	return nil
}

func importVolumes(source map[string]interface{}) ([]*volume, error) {
	checker := versionedChecker("volumes")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "volumes version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := volumeDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["volumes"].([]interface{})
	return importVolumeList(sourceList, importFunc)
}

func importVolumeList(sourceList []interface{}, importFunc volumeDeserializationFunc) ([]*volume, error) {
	result := make([]*volume, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for volume %d, %T", i, value)
		}
		volume, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "volume %d", i)
		}
		result = append(result, volume)
	}
	return result, nil
}

type volumeDeserializationFunc func(map[string]interface{}) (*volume, error)

var volumeDeserializationFuncs = map[int]volumeDeserializationFunc{
	1: importVolumeV1,
}

func importVolumeV1(source map[string]interface{}) (*volume, error) {
	fields := schema.Fields{
		"id":          schema.String(),
		"storage-id":  schema.String(),
		"binding":     schema.String(),
		"provisioned": schema.Bool(),
		"size":        schema.ForceUint(),
		"pool":        schema.String(),
		"hardware-id": schema.String(),
		"volume-id":   schema.String(),
		"persistent":  schema.Bool(),
		"status":      schema.StringMap(schema.Any()),
		"attachments": schema.StringMap(schema.Any()),
	}

	defaults := schema.Defaults{
		"storage-id":  "",
		"binding":     "",
		"pool":        "",
		"hardware-id": "",
		"volume-id":   "",
		"attachments": schema.Omit,
	}
	addStatusHistorySchema(fields)
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "volume v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &volume{
		ID_:            valid["id"].(string),
		StorageID_:     valid["storage-id"].(string),
		Binding_:       valid["binding"].(string),
		Provisioned_:   valid["provisioned"].(bool),
		Size_:          valid["size"].(uint64),
		Pool_:          valid["pool"].(string),
		HardwareID_:    valid["hardware-id"].(string),
		VolumeID_:      valid["volume-id"].(string),
		Persistent_:    valid["persistent"].(bool),
		StatusHistory_: newStatusHistory(),
	}
	if err := result.importStatusHistory(valid); err != nil {
		return nil, errors.Trace(err)
	}

	status, err := importStatus(valid["status"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Status_ = status

	attachments, err := importVolumeAttachments(valid["attachments"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.setAttachments(attachments)

	return result, nil
}

// VolumeAttachmentArgs is an argument struct used to add information about the
// cloud instance to a Volume.
type VolumeAttachmentArgs struct {
	Machine     names.MachineTag
	Provisioned bool
	ReadOnly    bool
	DeviceName  string
	DeviceLink  string
	BusAddress  string
}

func newVolumeAttachment(args VolumeAttachmentArgs) *volumeAttachment {
	return &volumeAttachment{
		MachineID_:   args.Machine.Id(),
		Provisioned_: args.Provisioned,
		ReadOnly_:    args.ReadOnly,
		DeviceName_:  args.DeviceName,
		DeviceLink_:  args.DeviceLink,
		BusAddress_:  args.BusAddress,
	}
}

// Machine implements VolumeAttachment
func (a *volumeAttachment) Machine() names.MachineTag {
	return names.NewMachineTag(a.MachineID_)
}

// Provisioned implements VolumeAttachment
func (a *volumeAttachment) Provisioned() bool {
	return a.Provisioned_
}

// ReadOnly implements VolumeAttachment
func (a *volumeAttachment) ReadOnly() bool {
	return a.ReadOnly_
}

// DeviceName implements VolumeAttachment
func (a *volumeAttachment) DeviceName() string {
	return a.DeviceName_
}

// DeviceLink implements VolumeAttachment
func (a *volumeAttachment) DeviceLink() string {
	return a.DeviceLink_
}

// BusAddress implements VolumeAttachment
func (a *volumeAttachment) BusAddress() string {
	return a.BusAddress_
}

func importVolumeAttachments(source map[string]interface{}) ([]*volumeAttachment, error) {
	checker := versionedChecker("attachments")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "volume attachments version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := volumeAttachmentDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["attachments"].([]interface{})
	return importVolumeAttachmentList(sourceList, importFunc)
}

func importVolumeAttachmentList(sourceList []interface{}, importFunc volumeAttachmentDeserializationFunc) ([]*volumeAttachment, error) {
	result := make([]*volumeAttachment, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for volumeAttachment %d, %T", i, value)
		}
		volumeAttachment, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "volumeAttachment %d", i)
		}
		result = append(result, volumeAttachment)
	}
	return result, nil
}

type volumeAttachmentDeserializationFunc func(map[string]interface{}) (*volumeAttachment, error)

var volumeAttachmentDeserializationFuncs = map[int]volumeAttachmentDeserializationFunc{
	1: importVolumeAttachmentV1,
}

func importVolumeAttachmentV1(source map[string]interface{}) (*volumeAttachment, error) {
	fields := schema.Fields{
		"machine-id":  schema.String(),
		"provisioned": schema.Bool(),
		"read-only":   schema.Bool(),
		"device-name": schema.String(),
		"device-link": schema.String(),
		"bus-address": schema.String(),
	}
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "volumeAttachment v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &volumeAttachment{
		MachineID_:   valid["machine-id"].(string),
		Provisioned_: valid["provisioned"].(bool),
		ReadOnly_:    valid["read-only"].(bool),
		DeviceName_:  valid["device-name"].(string),
		DeviceLink_:  valid["device-link"].(string),
		BusAddress_:  valid["bus-address"].(string),
	}
	return result, nil
}
