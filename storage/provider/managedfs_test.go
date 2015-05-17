// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
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
	s.commands.expect("mkfs.ext4", "/dev/sda")
	s.commands.expect("mkfs.ext4", "/dev/disk/by-id/weetbix")

	s.blockDevices[names.NewVolumeTag("0")] = storage.BlockDevice{
		DeviceName: "sda",
		HardwareId: "capncrunch",
		Size:       2,
	}
	s.blockDevices[names.NewVolumeTag("1")] = storage.BlockDevice{
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
		Tag:          names.NewFilesystemTag("0/0"),
		Volume:       names.NewVolumeTag("0"),
		FilesystemId: "filesystem-0-0",
		Size:         2,
	}, {
		Tag:          names.NewFilesystemTag("0/1"),
		Volume:       names.NewVolumeTag("1"),
		FilesystemId: "filesystem-0-1",
		Size:         3,
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
	source := s.initSource(c)
	s.commands.expect("mount", "/dev/sda", "/in/the/place")

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
		},
		Path: "/in/the/place",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("0/0"),
		Machine:    names.NewMachineTag("0"),
		Path:       "/in/the/place",
	}})
}

func (s *managedfsSuite) TestDetachFilesystems(c *gc.C) {
	source := s.initSource(c)
	err := source.DetachFilesystems(nil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}
