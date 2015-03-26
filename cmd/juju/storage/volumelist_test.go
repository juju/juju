// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

	s.mockAPI = &mockVolumeListAPI{fillDeviceName: true}
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

	context, err := runVolumeList(c, []string{"--format", "yaml"})
	c.Assert(errors.Cause(err), gc.ErrorMatches, s.mockAPI.errOut)
	s.assertUserFacingOutput(c, context, "", "")
}

func (s *volumeListSuite) TestVolumeListAll(c *gc.C) {
	s.mockAPI.listAll = true
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
"":
  3/3:
    serial: serial blah blah
    size: 1024
    persistent: false
    readonly: false
  3/4:
    serial: serial blah blah
    size: 1024
    persistent: true
    readonly: false
"25":
  0/1:
    serial: serial blah blah
    size: 1024
    persistent: true
    device-name: testdevice
    readonly: true
  0/abc/0/88:
    serial: serial blah blah
    size: 1024
    persistent: false
    device-name: testdevice
    readonly: true
"42":
  0/1:
    serial: serial blah blah
    size: 1024
    persistent: true
    device-name: testdevice
    readonly: false
  0/abc/0/88:
    serial: serial blah blah
    size: 1024
    persistent: false
    device-name: testdevice
    readonly: false
`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListYaml(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
"2":
  0/1:
    serial: serial blah blah
    size: 1024
    persistent: true
    device-name: testdevice
    readonly: true
  0/abc/0/88:
    serial: serial blah blah
    size: 1024
    persistent: false
    device-name: testdevice
    readonly: true
`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListYamlNoDeviceName(c *gc.C) {
	s.mockAPI.fillDeviceName = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
"2":
  0/1:
    serial: serial blah blah
    size: 1024
    persistent: true
    readonly: true
  0/abc/0/88:
    serial: serial blah blah
    size: 1024
    persistent: false
    readonly: true
`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "--format", "json"},
		`
{"2":{"0/1":{"serial":"serial blah blah","size":1024,"persistent":true,"device-name":"testdevice","readonly":true},"0/abc/0/88":{"serial":"serial blah blah","size":1024,"persistent":false,"device-name":"testdevice","readonly":true}}}
`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
MACHINE  DEVICE_NAME  VOLUME      SIZE
2        testdevice   0/1         1.0GiB
2        testdevice   0/abc/0/88  1.0GiB

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
MACHINE  DEVICE_NAME  VOLUME      SIZE
2        testdevice   0/1         1.0GiB
2        testdevice   0/abc/0/88  1.0GiB
3        testdevice   0/1         1.0GiB
3        testdevice   0/abc/0/88  1.0GiB

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
MACHINE  DEVICE_NAME  VOLUME      SIZE
                      3/3         1.0GiB
                      3/4         1.0GiB
25       testdevice   0/1         1.0GiB
25       testdevice   0/abc/0/88  1.0GiB
42       testdevice   0/1         1.0GiB
42       testdevice   0/abc/0/88  1.0GiB

`[1:],
		`
volume item error
`[1:],
	)
}

func (s *volumeListSuite) assertValidList(c *gc.C, args []string, expectedOut, expectedErr string) {
	context, err := runVolumeList(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUserFacingOutput(c, context, expectedOut, expectedErr)
}

func runVolumeList(c *gc.C, args []string) (*cmd.Context, error) {
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
	listAll, listEmpty, fillDeviceName bool
	errOut                             string
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
	result := []params.VolumeItem{
		params.VolumeItem{Error: common.ServerError(errors.New("volume item error"))}}
	if s.listAll {
		machines = []string{"25", "42"}
		//unattached
		result = append(result, s.createTestVolumeItem("3/4", true, nil))
		result = append(result, s.createTestVolumeItem("3/3", false, nil))
	}
	result = append(result, s.createTestVolumeItem("0/1", true, machines))
	result = append(result, s.createTestVolumeItem("0/abc/0/88", false, machines))
	return result, nil
}

func (s mockVolumeListAPI) createTestVolumeItem(id string, persistent bool, machines []string) params.VolumeItem {
	volume := s.createTestVolume(id, persistent)

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

func (s mockVolumeListAPI) createTestVolume(id string, persistent bool) params.Volume {
	tag := names.NewVolumeTag(id)
	result := params.Volume{
		VolumeTag:  tag.String(),
		VolumeId:   tag.Id(),
		Serial:     "serial blah blah",
		Persistent: persistent,
		Size:       uint64(1024),
	}
	return result
}

func (s mockVolumeListAPI) createTestAttachment(volumeTag, machine string, readonly bool) params.VolumeAttachment {
	result := params.VolumeAttachment{
		VolumeTag:  volumeTag,
		MachineTag: names.NewMachineTag(machine).String(),
		ReadOnly:   readonly,
	}
	if s.fillDeviceName {
		result.DeviceName = "testdevice"
	}
	return result
}
