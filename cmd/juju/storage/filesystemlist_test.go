// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"encoding/json"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/testing"
)

type filesystemListSuite struct {
	SubStorageSuite
	mockAPI *mockFilesystemListAPI
}

var _ = gc.Suite(&filesystemListSuite{})

func (s *filesystemListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockFilesystemListAPI{fillMountPoint: true, addErrItem: true}
	s.PatchValue(storage.GetFilesystemListAPI,
		func(c *storage.FilesystemListCommand) (storage.FilesystemListAPI, error) {
			return s.mockAPI, nil
		})
}

func (s *filesystemListSuite) TestFilesystemListEmpty(c *gc.C) {
	s.mockAPI.listEmpty = true
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		"",
		"",
	)
}

func (s *filesystemListSuite) TestFilesystemListError(c *gc.C) {
	s.mockAPI.errOut = "just my luck"

	context, err := runFilesystemList(c, "--format", "yaml")
	c.Assert(errors.Cause(err), gc.ErrorMatches, s.mockAPI.errOut)
	s.assertUserFacingOutput(c, context, "", "")
}

func (s *filesystemListSuite) TestFilesystemListAll(c *gc.C) {
	s.mockAPI.listAll = true
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		// mock will ignore any value here, as listAll flag above has precedence
		"",
		"--format", "yaml")
}

func (s *filesystemListSuite) TestFilesystemListYaml(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"2",
		"--format", "yaml")
}

func (s *filesystemListSuite) TestFilesystemListYamlNoMountPoint(c *gc.C) {
	s.mockAPI.fillMountPoint = false
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"2",
		"--format", "yaml")
}

func (s *filesystemListSuite) TestFilesystemListJSON(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		json.Unmarshal,
		"2",
		"--format", "json")
}

func (s *filesystemListSuite) TestFilesystemListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
MACHINE  UNIT          STORAGE      FILESYSTEM  VOLUME  ID                            MOUNTPOINT  SIZE    STATE      MESSAGE
2        postgresql/0  shared-fs/0  0/1         0/99    provider-supplied-0/1         testmntpnt  1.0GiB  attaching  failed to attach
2        unattached    shared-fs/0  0/abc/0/88          provider-supplied-0/abc/0/88  testmntpnt  1.0GiB  attached   

`[1:],
		`
filesystem item error
`[1:],
	)
}

func (s *filesystemListSuite) TestFilesystemListTabularSort(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE  UNIT          STORAGE      FILESYSTEM  VOLUME  ID                            MOUNTPOINT  SIZE    STATE      MESSAGE
2        postgresql/0  shared-fs/0  0/1         0/99    provider-supplied-0/1         testmntpnt  1.0GiB  attaching  failed to attach
2        unattached    shared-fs/0  0/abc/0/88          provider-supplied-0/abc/0/88  testmntpnt  1.0GiB  attached   
3        postgresql/0  shared-fs/0  0/1         0/99    provider-supplied-0/1         testmntpnt  1.0GiB  attaching  failed to attach
3        unattached    shared-fs/0  0/abc/0/88          provider-supplied-0/abc/0/88  testmntpnt  1.0GiB  attached   

`[1:],
		`
filesystem item error
`[1:],
	)
}

func (s *filesystemListSuite) TestFilesystemListTabularSortWithUnattached(c *gc.C) {
	s.mockAPI.listAll = true
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE     UNIT          STORAGE      FILESYSTEM  VOLUME  ID                            MOUNTPOINT  SIZE    STATE       MESSAGE
25          postgresql/0  shared-fs/0  0/1         0/99    provider-supplied-0/1         testmntpnt  1.0GiB  attaching   failed to attach
25          unattached    shared-fs/0  0/abc/0/88          provider-supplied-0/abc/0/88  testmntpnt  1.0GiB  attached    
42          postgresql/0  shared-fs/0  0/1         0/99    provider-supplied-0/1         testmntpnt  1.0GiB  attaching   failed to attach
42          unattached    shared-fs/0  0/abc/0/88          provider-supplied-0/abc/0/88  testmntpnt  1.0GiB  attached    
unattached  abc/0         db-dir/1000  3/4                 provider-supplied-3/4                     1.0GiB  destroying  
unattached  unattached    unassigned   3/3                 provider-supplied-3/3                     1.0GiB  destroying  

`[1:],
		`
filesystem item error
`[1:],
	)
}

func (s *filesystemListSuite) assertUnmarshalledOutput(c *gc.C, unmarshall unmarshaller, machine string, args ...string) {
	all := []string{machine}
	context, err := runFilesystemList(c, append(all, args...)...)
	c.Assert(err, jc.ErrorIsNil)
	var result map[string]map[string]map[string]storage.FilesystemInfo
	err = unmarshall(context.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	expected := s.expect(c, []string{machine})
	c.Assert(result, jc.DeepEquals, expected)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, `
filesystem item error
`[1:])
}

func (s *filesystemListSuite) expect(c *gc.C, machines []string) map[string]map[string]map[string]storage.FilesystemInfo {
	//no need for this element as we are building output on out stream not err
	s.mockAPI.addErrItem = false
	all, err := s.mockAPI.ListFilesystems(machines)
	c.Assert(err, jc.ErrorIsNil)
	result, err := storage.ConvertToFilesystemInfo(all)
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func (s *filesystemListSuite) assertValidList(c *gc.C, args []string, expectedOut, expectedErr string) {
	context, err := runFilesystemList(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUserFacingOutput(c, context, expectedOut, expectedErr)
}

func runFilesystemList(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c,
		envcmd.Wrap(&storage.FilesystemListCommand{}),
		args...)
}

func (s *filesystemListSuite) assertUserFacingOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedOut := testing.Stdout(context)
	c.Assert(obtainedOut, gc.Equals, expectedOut)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)
}

type mockFilesystemListAPI struct {
	listAll, listEmpty, fillMountPoint, addErrItem bool
	errOut                                         string
}

func (s mockFilesystemListAPI) Close() error {
	return nil
}

func (s mockFilesystemListAPI) ListFilesystems(machines []string) ([]params.FilesystemDetailsResult, error) {
	if s.errOut != "" {
		return nil, errors.New(s.errOut)
	}
	if s.listEmpty {
		return nil, nil
	}
	result := []params.FilesystemDetailsResult{}
	if s.addErrItem {
		result = append(result, params.FilesystemDetailsResult{
			Error: common.ServerError(errors.New("filesystem item error"))})
	}
	if s.listAll {
		machines = []string{"25", "42"}
		//unattached
		result = append(result, s.createTestFilesystemDetailsResult(
			"3/4", "", true, "db-dir/1000", "abc/0", nil,
			createTestStatus(params.StatusDestroying, ""),
		))
		result = append(result, s.createTestFilesystemDetailsResult(
			"3/3", "", false, "", "", nil,
			createTestStatus(params.StatusDestroying, ""),
		))
	}
	result = append(result, s.createTestFilesystemDetailsResult(
		"0/1", "0/99", true, "shared-fs/0", "postgresql/0", machines,
		createTestStatus(params.StatusAttaching, "failed to attach"),
	))
	result = append(result, s.createTestFilesystemDetailsResult(
		"0/abc/0/88", "", false, "shared-fs/0", "", machines,
		createTestStatus(params.StatusAttached, ""),
	))
	return result, nil
}

func (s mockFilesystemListAPI) createTestFilesystemDetailsResult(
	filesystemId, volumeId string,
	persistent bool,
	storageid, unitid string,
	machines []string,
	status params.EntityStatus,
) params.FilesystemDetailsResult {

	filesystem := s.createTestFilesystem(filesystemId, volumeId, persistent, storageid, unitid, status)
	filesystem.MachineAttachments = make(map[string]params.FilesystemAttachmentInfo)
	for i, machine := range machines {
		info := params.FilesystemAttachmentInfo{
			ReadOnly: i%2 == 0,
		}
		if s.fillMountPoint {
			info.MountPoint = "testmntpnt"
		}
		machineTag := names.NewMachineTag(machine).String()
		filesystem.MachineAttachments[machineTag] = info
	}
	return params.FilesystemDetailsResult{Result: filesystem}
}

func (s mockFilesystemListAPI) createTestFilesystem(
	filesystemId, volumeId string, persistent bool, storageid, unitid string, status params.EntityStatus,
) *params.FilesystemDetails {
	tag := names.NewFilesystemTag(filesystemId)
	result := &params.FilesystemDetails{
		FilesystemTag: tag.String(),
		Info: params.FilesystemInfo{
			FilesystemId: "provider-supplied-" + tag.Id(),
			Size:         uint64(1024),
		},
		Status: status,
	}
	if volumeId != "" {
		result.VolumeTag = names.NewVolumeTag(volumeId).String()
	}
	if storageid != "" {
		result.StorageTag = names.NewStorageTag(storageid).String()
	}
	if unitid != "" {
		result.StorageOwnerTag = names.NewUnitTag(unitid).String()
	}
	return result
}
