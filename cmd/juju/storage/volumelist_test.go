// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"fmt"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/testing"
)

type volumeListSuite struct {
	SubStorageSuite
	mockAPI *mockVolumeListAPI
}

var _ = gc.Suite(&volumeListSuite{})

func (s *volumeListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockVolumeListAPI{fillDeviceName: true, addErrItem: true}
	s.PatchValue(storage.GetVolumeListAPI,
		func(c *storage.VolumeListCommand) (storage.VolumeListAPI, error) {
			return s.mockAPI, nil
		})
}

func (s *volumeListSuite) TestVolumeListEmpty(c *gc.C) {
	s.mockAPI.listEmpty = true
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		"",
		"",
	)
}

func (s *volumeListSuite) TestVolumeListError(c *gc.C) {
	s.mockAPI.errOut = "just my luck"

	context, err := runVolumeList(c, "--format", "yaml")
	c.Assert(errors.Cause(err), gc.ErrorMatches, s.mockAPI.errOut)
	s.assertUserFacingOutput(c, context, "", "")
}

func (s *volumeListSuite) TestVolumeListAll(c *gc.C) {
	s.mockAPI.listAll = true
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		// mock will ignore any value here, as listAll flag above has precedence
		"",
		"--format", "yaml")
}

func (s *volumeListSuite) TestVolumeListYaml(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"2",
		"--format", "yaml")
}

func (s *volumeListSuite) TestVolumeListYamlNoDeviceName(c *gc.C) {
	s.mockAPI.fillDeviceName = false
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"2",
		"--format", "yaml")
}

func (s *volumeListSuite) TestVolumeListJSON(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		json.Unmarshal,
		"2",
		"--format", "json")
}

func (s *volumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
MACHINE  UNIT          STORAGE      DEVICE      VOLUME      ID                            SIZE    STATE      MESSAGE
2        postgresql/0  shared-fs/0  testdevice  0/1         provider-supplied-0/1         1.0GiB  attaching  failed to attach
2        unattached    shared-fs/0  testdevice  0/abc/0/88  provider-supplied-0/abc/0/88  1.0GiB  attached   

`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListTabularSort(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE  UNIT          STORAGE      DEVICE      VOLUME      ID                            SIZE    STATE      MESSAGE
2        postgresql/0  shared-fs/0  testdevice  0/1         provider-supplied-0/1         1.0GiB  attaching  failed to attach
2        unattached    shared-fs/0  testdevice  0/abc/0/88  provider-supplied-0/abc/0/88  1.0GiB  attached   
3        postgresql/0  shared-fs/0  testdevice  0/1         provider-supplied-0/1         1.0GiB  attaching  failed to attach
3        unattached    shared-fs/0  testdevice  0/abc/0/88  provider-supplied-0/abc/0/88  1.0GiB  attached   

`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListTabularSortWithUnattached(c *gc.C) {
	s.mockAPI.listAll = true
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE     UNIT          STORAGE      DEVICE      VOLUME      ID                            SIZE    STATE       MESSAGE
25          postgresql/0  shared-fs/0  testdevice  0/1         provider-supplied-0/1         1.0GiB  attaching   failed to attach
25          unattached    shared-fs/0  testdevice  0/abc/0/88  provider-supplied-0/abc/0/88  1.0GiB  attached    
42          postgresql/0  shared-fs/0  testdevice  0/1         provider-supplied-0/1         1.0GiB  attaching   failed to attach
42          unattached    shared-fs/0  testdevice  0/abc/0/88  provider-supplied-0/abc/0/88  1.0GiB  attached    
unattached  abc/0         db-dir/1000              3/4         provider-supplied-3/4         1.0GiB  destroying  
unattached  unattached    unassigned               3/3         provider-supplied-3/3         1.0GiB  destroying  

`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) assertUnmarshalledOutput(c *gc.C, unmarshall unmarshaller, machine string, args ...string) {
	all := []string{machine}
	context, err := runVolumeList(c, append(all, args...)...)
	c.Assert(err, jc.ErrorIsNil)
	var result map[string]map[string]map[string]storage.VolumeInfo
	err = unmarshall(context.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	expected := s.expect(c, []string{machine})
	// This comparison cannot rely on gc.DeepEquals as
	// json.Unmarshal unmarshalls the number as a float64,
	// rather than an int
	s.assertSameVolumeInfos(c, result, expected)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, `
volume item error
`[1:])
}

func (s *volumeListSuite) expect(c *gc.C, machines []string) map[string]map[string]map[string]storage.VolumeInfo {
	//no need for this element as we are building output on out stream not err
	s.mockAPI.addErrItem = false
	all, err := s.mockAPI.ListVolumes(machines)
	c.Assert(err, jc.ErrorIsNil)
	result, err := storage.ConvertToVolumeInfo(all)
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func (s *volumeListSuite) assertSameVolumeInfos(c *gc.C, one, two map[string]map[string]map[string]storage.VolumeInfo) {
	c.Assert(len(one), gc.Equals, len(two))

	propertyCompare := func(a, b interface{}) {
		// As some types may have been unmarshalled incorrectly, for example
		// int versus float64, compare values' string representations
		c.Assert(fmt.Sprintf("%v", a), jc.DeepEquals, fmt.Sprintf("%v", b))

	}
	for machineKey, machineVolumes1 := range one {
		machineVolumes2, ok := two[machineKey]
		c.Assert(ok, jc.IsTrue)
		// these are maps
		c.Assert(len(machineVolumes1), gc.Equals, len(machineVolumes2))
		for unitKey, units1 := range machineVolumes1 {
			units2, ok := machineVolumes2[unitKey]
			c.Assert(ok, jc.IsTrue)
			// these are maps
			c.Assert(len(units1), gc.Equals, len(units2))
			for storageKey, info1 := range units1 {
				info2, ok := units2[storageKey]
				c.Assert(ok, jc.IsTrue)
				propertyCompare(info1.VolumeId, info2.VolumeId)
				propertyCompare(info1.HardwareId, info2.HardwareId)
				propertyCompare(info1.Size, info2.Size)
				propertyCompare(info1.Persistent, info2.Persistent)
				propertyCompare(info1.DeviceName, info2.DeviceName)
				propertyCompare(info1.ReadOnly, info2.ReadOnly)
			}
		}
	}
}

func (s *volumeListSuite) assertValidList(c *gc.C, args []string, expectedOut, expectedErr string) {
	context, err := runVolumeList(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUserFacingOutput(c, context, expectedOut, expectedErr)
}

func runVolumeList(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c,
		envcmd.Wrap(&storage.VolumeListCommand{}),
		args...)
}

func (s *volumeListSuite) assertUserFacingOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedOut := testing.Stdout(context)
	c.Assert(obtainedOut, gc.Equals, expectedOut)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)
}

type mockVolumeListAPI struct {
	listAll, listEmpty, fillDeviceName, addErrItem bool
	errOut                                         string
}

func (s mockVolumeListAPI) Close() error {
	return nil
}

func (s mockVolumeListAPI) ListVolumes(machines []string) ([]params.VolumeItem, error) {
	if s.errOut != "" {
		return nil, errors.New(s.errOut)
	}
	if s.listEmpty {
		return nil, nil
	}
	result := []params.VolumeItem{}
	if s.addErrItem {
		result = append(result, params.VolumeItem{
			Error: common.ServerError(errors.New("volume item error"))})
	}
	if s.listAll {
		machines = []string{"25", "42"}
		//unattached
		result = append(result, s.createTestVolumeItem(
			"3/4", true, "db-dir/1000", "abc/0", nil,
			createTestStatus(params.StatusDestroying, ""),
		))
		result = append(result, s.createTestVolumeItem(
			"3/3", false, "", "", nil,
			createTestStatus(params.StatusDestroying, ""),
		))
	}
	result = append(result, s.createTestVolumeItem(
		"0/1", true, "shared-fs/0", "postgresql/0", machines,
		createTestStatus(params.StatusAttaching, "failed to attach"),
	))
	result = append(result, s.createTestVolumeItem(
		"0/abc/0/88", false, "shared-fs/0", "", machines,
		createTestStatus(params.StatusAttached, ""),
	))
	return result, nil
}

func (s mockVolumeListAPI) createTestVolumeItem(
	id string,
	persistent bool,
	storageid, unitid string,
	machines []string,
	status params.EntityStatus,
) params.VolumeItem {
	volume := s.createTestVolume(id, persistent, storageid, unitid, status)

	// Create unattached volume
	if len(machines) == 0 {
		return params.VolumeItem{Volume: volume}
	}

	// Create volume attachments
	attachments := make([]params.VolumeAttachment, len(machines))
	for i, machine := range machines {
		attachments[i] = s.createTestAttachment(volume.VolumeTag, machine, i%2 == 0)
	}

	return params.VolumeItem{
		Volume:      volume,
		Attachments: attachments,
	}
}

func (s mockVolumeListAPI) createTestVolume(id string, persistent bool, storageid, unitid string, status params.EntityStatus) params.VolumeInstance {
	tag := names.NewVolumeTag(id)
	result := params.VolumeInstance{
		VolumeTag:  tag.String(),
		VolumeId:   "provider-supplied-" + tag.Id(),
		HardwareId: "serial blah blah",
		Persistent: persistent,
		Size:       uint64(1024),
		Status:     status,
	}
	if storageid != "" {
		result.StorageTag = names.NewStorageTag(storageid).String()
	}
	if unitid != "" {
		result.UnitTag = names.NewUnitTag(unitid).String()
	}
	return result
}

func (s mockVolumeListAPI) createTestAttachment(volumeTag, machine string, readonly bool) params.VolumeAttachment {
	result := params.VolumeAttachment{
		VolumeTag:  volumeTag,
		MachineTag: names.NewMachineTag(machine).String(),
		Info: params.VolumeAttachmentInfo{
			ReadOnly: readonly,
		},
	}
	if s.fillDeviceName {
		result.Info.DeviceName = "testdevice"
	}
	return result
}

func createTestStatus(status params.Status, message string) params.EntityStatus {
	return params.EntityStatus{
		Status: status,
		Info:   message,
		Since:  &time.Time{},
	}
}
