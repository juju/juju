// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
)

func TestManagedfsSuite(t *stdtesting.T) {
	tc.Run(t, &managedfsSuite{})
}

type managedfsSuite struct {
	testing.BaseSuite
	commands     *mockRunCommand
	dirFuncs     *provider.MockDirFuncs
	blockDevices map[names.VolumeTag]blockdevice.BlockDevice
	filesystems  map[names.FilesystemTag]storage.Filesystem
	fakeEtcDir   string
}

func (s *managedfsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.blockDevices = make(map[names.VolumeTag]blockdevice.BlockDevice)
	s.filesystems = make(map[names.FilesystemTag]storage.Filesystem)
	s.fakeEtcDir = c.MkDir()
}

func (s *managedfsSuite) TearDownTest(c *tc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *managedfsSuite) initSource(c *tc.C, fakeMountInfo ...string) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	source, mockDirFuncs := provider.NewMockManagedFilesystemSource(
		s.fakeEtcDir,
		s.commands.run,
		s.blockDevices,
		s.filesystems,
		fakeMountInfo...,
	)
	s.dirFuncs = mockDirFuncs
	return source
}

func (s *managedfsSuite) TestCreateFilesystems(c *tc.C) {
	source := s.initSource(c)
	// sda is (re)partitioned and the filesystem created
	// on the partition.
	s.commands.expect("sgdisk", "--zap-all", "/dev/sda")
	s.commands.expect("sgdisk", "-n", "1:0:-1", "/dev/sda")
	s.commands.expect("mkfs.ext4", "/dev/sda1")
	// xvdf1 is assumed to not require a partition, on
	// account of ending with a digit.
	s.commands.expect("mkfs.ext4", "/dev/xvdf1")

	s.blockDevices[names.NewVolumeTag("0")] = blockdevice.BlockDevice{
		DeviceName: "sda",
		HardwareId: "capncrunch",
		SizeMiB:    2,
	}
	s.blockDevices[names.NewVolumeTag("1")] = blockdevice.BlockDevice{
		DeviceName: "xvdf1",
		HardwareId: "weetbix",
		SizeMiB:    3,
	}
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}, {
		Tag:    names.NewFilesystemTag("0/1"),
		Volume: names.NewVolumeTag("1"),
		Size:   3,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.CreateFilesystemsResult{{
		Filesystem: &storage.Filesystem{
			names.NewFilesystemTag("0/0"),
			names.NewVolumeTag("0"),
			storage.FilesystemInfo{
				FilesystemId: "filesystem-0-0",
				Size:         2,
			},
		},
	}, {
		Filesystem: &storage.Filesystem{
			names.NewFilesystemTag("0/1"),
			names.NewVolumeTag("1"),
			storage.FilesystemInfo{
				FilesystemId: "filesystem-0-1",
				Size:         3,
			},
		},
	}})
}

func (s *managedfsSuite) TestCreateFilesystemsNoBlockDevice(c *tc.C) {
	source := s.initSource(c)
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "backing-volume 0 is not yet attached")
}

const testMountPoint = "/in/the/place"

func mountInfoLine(id, parent int, root, mountPoint, source string) string {
	return fmt.Sprintf("%d %d 8:1 %s %s rw,relatime shared:1 - ext4 %s rw,errors=remount-ro", id, parent, root, mountPoint, source)
}

func (s *managedfsSuite) TestAttachFilesystems(c *tc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff"
	err := os.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry), 0644)
	c.Assert(err, tc.ErrorIsNil)

	mtabEntry := fmt.Sprintf("/dev/sda1 %s other relatime 0 0", testMountPoint)
	fstabEntry := fmt.Sprintf("/dev/sda1 %s other nofail,relatime 0 0", testMountPoint)
	s.testAttachFilesystems(c, false, false, "", mtabEntry, nonRelatedFstabEntry+"\n"+fstabEntry+"\n")
}

func (s *managedfsSuite) TestAttachFilesystemsMissingMtab(c *tc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff\n"
	err := os.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry), 0644)
	c.Assert(err, tc.ErrorIsNil)

	s.testAttachFilesystems(c, false, false, "", "", nonRelatedFstabEntry)
}

func (s *managedfsSuite) TestAttachFilesystemsExistingFstabEntry(c *tc.C) {
	existingFstabEntry := fmt.Sprintf("/dev/sda1 %s existing mtab stuff\n", testMountPoint)
	err := os.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(existingFstabEntry), 0644)
	c.Assert(err, tc.ErrorIsNil)

	mtabEntry := fmt.Sprintf("/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, false, false, "", mtabEntry, existingFstabEntry)
}

func (s *managedfsSuite) TestAttachFilesystemsUpdateExistingFstabEntryWithUUID(c *tc.C) {
	existingFstabEntry := fmt.Sprintf("/dev/sda1 %s existing mtab stuff\n", testMountPoint)
	err := os.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(existingFstabEntry), 0644)
	c.Assert(err, tc.ErrorIsNil)

	expectedFstabEntry := fmt.Sprintf("# %s was on /dev/sda1 during installation\nUUID=deadbeaf %s other mtab,nofail stuff\n", testMountPoint, testMountPoint)
	mtabEntry := fmt.Sprintf("/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, false, false, "deadbeaf", mtabEntry, expectedFstabEntry)
}

func (s *managedfsSuite) TestAttachFilesystemsReadOnly(c *tc.C) {
	mtabEntry := fmt.Sprintf("/dev/sda1 %s other nofail,relatime 0 0", testMountPoint)
	s.testAttachFilesystems(c, true, false, "", mtabEntry, mtabEntry+"\n")
}

func (s *managedfsSuite) TestAttachFilesystemsReattach(c *tc.C) {
	mtabEntry := fmt.Sprintf("/dev/sda1 %s other nofail,relatime 0 0", testMountPoint)
	s.testAttachFilesystems(c, true, true, "", mtabEntry, "")
}

func (s *managedfsSuite) testAttachFilesystems(c *tc.C, readOnly, reattach bool, uuid, mtab, fstab string) {
	mountInfo := ""
	if reattach {
		mountInfo = mountInfoLine(666, 0, "/different/to/rootfs", testMountPoint, "/dev/sda1")
	}
	source := s.initSource(c, mountInfo)

	if mtab != "" {
		err := os.WriteFile(filepath.Join(s.fakeEtcDir, "mtab"), []byte(mtab), 0644)
		c.Assert(err, tc.ErrorIsNil)
	}

	if !reattach {
		var args []string
		if readOnly {
			args = append(args, "-o", "ro")
		}
		args = append(args, "/dev/sda1", testMountPoint)
		s.commands.expect("mount", args...)
	}

	s.blockDevices[names.NewVolumeTag("0")] = blockdevice.BlockDevice{
		DeviceName: "sda",
		HardwareId: "capncrunch",
		SizeMiB:    2,
		UUID:       uuid,
	}
	s.filesystems[names.NewFilesystemTag("0/0")] = storage.Filesystem{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
	}

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("0/0"),
		FilesystemId: "filesystem-0-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: "inst-ance",
			ReadOnly:   readOnly,
		},
		Path: testMountPoint,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("0/0"),
			Machine:    names.NewMachineTag("0"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path:     testMountPoint,
				ReadOnly: readOnly,
			},
		},
	}})

	if fstab != "" {
		data, err := os.ReadFile(filepath.Join(s.fakeEtcDir, "fstab"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(data), tc.Equals, fstab)
	}
}

func (s *managedfsSuite) TestDetachFilesystems(c *tc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff\n"
	fstabEntry := fmt.Sprintf("%s %s other mtab stuff", "/dev/sda1", testMountPoint)
	err := os.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry+fstabEntry), 0644)
	c.Assert(err, tc.ErrorIsNil)
	mountInfo := mountInfoLine(666, 0, "/same/as/rootfs", testMountPoint, "/dev/sda1")
	source := s.initSource(c, mountInfo)
	testDetachFilesystems(c, s.commands, source, true, s.fakeEtcDir, nonRelatedFstabEntry)
}

func (s *managedfsSuite) TestDetachFilesystemsUnattached(c *tc.C) {
	source := s.initSource(c)
	testDetachFilesystems(c, s.commands, source, false, s.fakeEtcDir, "")
}
