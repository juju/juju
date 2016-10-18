// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"
)

type VolumeSerializationSuite struct {
	SliceSerializationSuite
	StatusHistoryMixinSuite
}

var _ = gc.Suite(&VolumeSerializationSuite{})

func (s *VolumeSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "volumes"
	s.sliceName = "volumes"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importVolumes(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["volumes"] = []interface{}{}
	}
	s.StatusHistoryMixinSuite.creator = func() HasStatusHistory {
		return testVolume()
	}
	s.StatusHistoryMixinSuite.serializer = func(c *gc.C, initial interface{}) HasStatusHistory {
		return s.exportImport(c, initial.(*volume))
	}
}

func testVolumeMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"id":             "1234",
		"storage-id":     "test/1",
		"binding":        "machine-42",
		"provisioned":    true,
		"size":           int(20 * gig),
		"pool":           "swimming",
		"hardware-id":    "a hardware id",
		"volume-id":      "some volume id",
		"persistent":     true,
		"status":         minimalStatusMap(),
		"status-history": emptyStatusHistoryMap(),
		"attachments": map[interface{}]interface{}{
			"version":     1,
			"attachments": []interface{}{},
		},
	}
}

func testVolume() *volume {
	v := newVolume(testVolumeArgs())
	v.SetStatus(minimalStatusArgs())
	return v
}

func testVolumeArgs() VolumeArgs {
	return VolumeArgs{
		Tag:         names.NewVolumeTag("1234"),
		Storage:     names.NewStorageTag("test/1"),
		Binding:     names.NewMachineTag("42"),
		Provisioned: true,
		Size:        20 * gig,
		Pool:        "swimming",
		HardwareID:  "a hardware id",
		VolumeID:    "some volume id",
		Persistent:  true,
	}
}

func (s *VolumeSerializationSuite) TestNewVolume(c *gc.C) {
	volume := testVolume()

	c.Check(volume.Tag(), gc.Equals, names.NewVolumeTag("1234"))
	c.Check(volume.Storage(), gc.Equals, names.NewStorageTag("test/1"))
	binding, err := volume.Binding()
	c.Check(err, jc.ErrorIsNil)
	c.Check(binding, gc.Equals, names.NewMachineTag("42"))
	c.Check(volume.Provisioned(), jc.IsTrue)
	c.Check(volume.Size(), gc.Equals, 20*gig)
	c.Check(volume.Pool(), gc.Equals, "swimming")
	c.Check(volume.HardwareID(), gc.Equals, "a hardware id")
	c.Check(volume.VolumeID(), gc.Equals, "some volume id")
	c.Check(volume.Persistent(), jc.IsTrue)

	c.Check(volume.Attachments(), gc.HasLen, 0)
}

func (s *VolumeSerializationSuite) TestVolumeValid(c *gc.C) {
	volume := testVolume()
	c.Assert(volume.Validate(), jc.ErrorIsNil)
}

func (s *VolumeSerializationSuite) TestVolumeValidMissingID(c *gc.C) {
	v := newVolume(VolumeArgs{})
	err := v.Validate()
	c.Check(err, gc.ErrorMatches, `volume missing id not valid`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *VolumeSerializationSuite) TestVolumeValidMissingSize(c *gc.C) {
	v := newVolume(VolumeArgs{
		Tag: names.NewVolumeTag("123"),
	})
	err := v.Validate()
	c.Check(err, gc.ErrorMatches, `volume "123" missing size not valid`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *VolumeSerializationSuite) TestVolumeValidMissingStatus(c *gc.C) {
	v := newVolume(VolumeArgs{
		Tag:  names.NewVolumeTag("123"),
		Size: 5,
	})
	err := v.Validate()
	c.Check(err, gc.ErrorMatches, `volume "123" missing status not valid`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *VolumeSerializationSuite) TestVolumeValidMinimal(c *gc.C) {
	v := newVolume(VolumeArgs{
		Tag:  names.NewVolumeTag("123"),
		Size: 5,
	})
	v.SetStatus(minimalStatusArgs())
	err := v.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (s *VolumeSerializationSuite) TestVolumeMatches(c *gc.C) {
	bytes, err := yaml.Marshal(testVolume())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, testVolumeMap())
}

func (s *VolumeSerializationSuite) exportImport(c *gc.C, volume_ *volume) *volume {
	initial := volumes{
		Version:  1,
		Volumes_: []*volume{volume_},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	volumes, err := importVolumes(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 1)
	return volumes[0]
}

func (s *VolumeSerializationSuite) TestAddingAttachments(c *gc.C) {
	// The core code does not care about duplicates, so we'll just add
	// the same attachment twice.
	original := testVolume()
	attachment1 := original.AddAttachment(testVolumeAttachmentArgs("1"))
	attachment2 := original.AddAttachment(testVolumeAttachmentArgs("2"))
	volume := s.exportImport(c, original)
	c.Assert(volume, jc.DeepEquals, original)
	attachments := volume.Attachments()
	c.Assert(attachments, gc.HasLen, 2)
	c.Check(attachments[0], jc.DeepEquals, attachment1)
	c.Check(attachments[1], jc.DeepEquals, attachment2)
}

func (s *VolumeSerializationSuite) TestParsingSerializedData(c *gc.C) {
	original := testVolume()
	original.AddAttachment(testVolumeAttachmentArgs())
	volume := s.exportImport(c, original)
	c.Assert(volume, jc.DeepEquals, original)
}

type VolumeAttachmentSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&VolumeAttachmentSerializationSuite{})

func (s *VolumeAttachmentSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "volume attachments"
	s.sliceName = "attachments"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importVolumeAttachments(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["attachments"] = []interface{}{}
	}
}

func testVolumeAttachmentMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"machine-id":  "42",
		"provisioned": true,
		"read-only":   true,
		"device-name": "sdd",
		"device-link": "link?",
		"bus-address": "nfi",
	}
}

func testVolumeAttachment() *volumeAttachment {
	return newVolumeAttachment(testVolumeAttachmentArgs())
}

func testVolumeAttachmentArgs(id ...string) VolumeAttachmentArgs {
	machineID := "42"
	if len(id) > 0 {
		machineID = id[0]
	}
	return VolumeAttachmentArgs{
		Machine:     names.NewMachineTag(machineID),
		Provisioned: true,
		ReadOnly:    true,
		DeviceName:  "sdd",
		DeviceLink:  "link?",
		BusAddress:  "nfi",
	}
}

func (s *VolumeAttachmentSerializationSuite) TestNewVolumeAttachment(c *gc.C) {
	attachment := testVolumeAttachment()

	c.Check(attachment.Machine(), gc.Equals, names.NewMachineTag("42"))
	c.Check(attachment.Provisioned(), jc.IsTrue)
	c.Check(attachment.ReadOnly(), jc.IsTrue)
	c.Check(attachment.DeviceName(), gc.Equals, "sdd")
	c.Check(attachment.DeviceLink(), gc.Equals, "link?")
	c.Check(attachment.BusAddress(), gc.Equals, "nfi")
}

func (s *VolumeAttachmentSerializationSuite) TestVolumeAttachmentMatches(c *gc.C) {
	bytes, err := yaml.Marshal(testVolumeAttachment())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, testVolumeAttachmentMap())
}

func (s *VolumeAttachmentSerializationSuite) exportImport(c *gc.C, attachment *volumeAttachment) *volumeAttachment {
	initial := volumeAttachments{
		Version:      1,
		Attachments_: []*volumeAttachment{attachment},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	attachments, err := importVolumeAttachments(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	return attachments[0]
}

func (s *VolumeAttachmentSerializationSuite) TestParsingSerializedData(c *gc.C) {
	original := testVolumeAttachment()
	attachment := s.exportImport(c, original)
	c.Assert(attachment, jc.DeepEquals, original)
}
