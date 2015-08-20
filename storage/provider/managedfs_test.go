// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&managedfsSuite{})

type managedfsSuite struct {
	testing.BaseSuite
	commands     *mockRunCommand
	dirFuncs     *provider.MockDirFuncs
	blockDevices map[names.VolumeTag]storage.BlockDevice
	filesystems  map[names.FilesystemTag]storage.Filesystem
}

func (s *managedfsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.blockDevices = make(map[names.VolumeTag]storage.BlockDevice)
	s.filesystems = make(map[names.FilesystemTag]storage.Filesystem)
}

func (s *managedfsSuite) TearDownTest(c *gc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *managedfsSuite) initSource(c *gc.C) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	source, mockDirFuncs := provider.NewMockManagedFilesystemSource(
		s.commands.run,
		s.blockDevices,
		s.filesystems,
	)
	s.dirFuncs = mockDirFuncs
	return source
}

func (s *managedfsSuite) TestCreateFilesystems(c *gc.C) {
	source := s.initSource(c)
	// sda is (re)partitioned and the filesystem created
	// on the partition.
	s.commands.expect("sgdisk", "--zap-all", "/dev/sda")
	s.commands.expect("sgdisk", "-n", "1:0:-1", "/dev/sda")
	s.commands.expect("mkfs.ext4", "/dev/sda1")
	// xvdf1 is assumed to not require a partition, on
	// account of ending with a digit.
	s.commands.expect("mkfs.ext4", "/dev/xvdf1")

	s.blockDevices[names.NewVolumeTag("0")] = storage.BlockDevice{
		DeviceName: "sda",
		HardwareId: "capncrunch",
		Size:       2,
	}
	s.blockDevices[names.NewVolumeTag("1")] = storage.BlockDevice{
		DeviceName: "xvdf1",
		HardwareId: "weetbix",
		Size:       3,
	}
	filesystems, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}, {
		Tag:    names.NewFilesystemTag("0/1"),
		Volume: names.NewVolumeTag("1"),
		Size:   3,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, jc.DeepEquals, []storage.Filesystem{{
		names.NewFilesystemTag("0/0"),
		names.NewVolumeTag("0"),
		storage.FilesystemInfo{
			FilesystemId: "filesystem-0-0",
			Size:         2,
		},
	}, {
		names.NewFilesystemTag("0/1"),
		names.NewVolumeTag("1"),
		storage.FilesystemInfo{
			FilesystemId: "filesystem-0-1",
			Size:         3,
		},
	}})
}

func (s *managedfsSuite) TestCreateFilesystemsNoBlockDevice(c *gc.C) {
	source := s.initSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}})
	c.Assert(err, gc.ErrorMatches, "creating filesystem 0/0: backing-volume 0 is not yet attached")
}

func (s *managedfsSuite) TestAttachFilesystems(c *gc.C) {
	s.testAttachFilesystems(c, false, false)
}

func (s *managedfsSuite) TestAttachFilesystemsReadOnly(c *gc.C) {
	s.testAttachFilesystems(c, true, false)
}

func (s *managedfsSuite) TestAttachFilesystemsReattach(c *gc.C) {
	s.testAttachFilesystems(c, true, true)
}

func (s *managedfsSuite) testAttachFilesystems(c *gc.C, readOnly, reattach bool) {
	const testMountPoint = "/in/the/place"

	source := s.initSource(c)
	cmd := s.commands.expect("df", "--output=source", filepath.Dir(testMountPoint))
	cmd.respond("headers\n/same/as/rootfs", nil)
	cmd = s.commands.expect("df", "--output=source", testMountPoint)
	if reattach {
		cmd.respond("headers\n/different/to/rootfs", nil)
	} else {
		cmd.respond("headers\n/same/as/rootfs", nil)
		var args []string
		if readOnly {
			args = append(args, "-o", "ro")
		}
		args = append(args, "/dev/sda1", testMountPoint)
		s.commands.expect("mount", args...)
	}

	s.blockDevices[names.NewVolumeTag("0")] = storage.BlockDevice{
		DeviceName: "sda",
		HardwareId: "capncrunch",
		Size:       2,
	}
	s.filesystems[names.NewFilesystemTag("0/0")] = storage.Filesystem{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
	}

	filesystemAttachments, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("0/0"),
		FilesystemId: "filesystem-0-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: "inst-ance",
			ReadOnly:   readOnly,
		},
		Path: testMountPoint,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, jc.DeepEquals, []storage.FilesystemAttachment{{
		names.NewFilesystemTag("0/0"),
		names.NewMachineTag("0"),
		storage.FilesystemAttachmentInfo{
			Path:     testMountPoint,
			ReadOnly: readOnly,
		},
	}})
}

func (s *managedfsSuite) TestDetachFilesystems(c *gc.C) {
	source := s.initSource(c)
	testDetachFilesystems(c, s.commands, source, true)
}

func (s *managedfsSuite) TestDetachFilesystemsUnattached(c *gc.C) {
	source := s.initSource(c)
	testDetachFilesystems(c, s.commands, source, false)
}
