// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/context"
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
	fakeEtcDir   string

	callCtx context.ProviderCallContext
}

func (s *managedfsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.blockDevices = make(map[names.VolumeTag]storage.BlockDevice)
	s.filesystems = make(map[names.FilesystemTag]storage.Filesystem)
	s.callCtx = context.NewCloudCallContext()
	s.fakeEtcDir = c.MkDir()
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
		s.fakeEtcDir,
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
	results, err := source.CreateFilesystems(s.callCtx, []storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}, {
		Tag:    names.NewFilesystemTag("0/1"),
		Volume: names.NewVolumeTag("1"),
		Size:   3,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []storage.CreateFilesystemsResult{{
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

func (s *managedfsSuite) TestCreateFilesystemsNoBlockDevice(c *gc.C) {
	source := s.initSource(c)
	results, err := source.CreateFilesystems(s.callCtx, []storage.FilesystemParams{{
		Tag:    names.NewFilesystemTag("0/0"),
		Volume: names.NewVolumeTag("0"),
		Size:   2,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results[0].Error, gc.ErrorMatches, "backing-volume 0 is not yet attached")
}

const testMountPoint = "/in/the/place"

func (s *managedfsSuite) TestAttachFilesystems(c *gc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff"
	err := ioutil.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry), 0644)
	c.Assert(err, jc.ErrorIsNil)

	mtabEntry := fmt.Sprintf("/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, false, false, mtabEntry, nonRelatedFstabEntry+"\n"+mtabEntry+"\n")
}

func (s *managedfsSuite) TestAttachFilesystemsMissingMtab(c *gc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff\n"
	err := ioutil.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.testAttachFilesystems(c, false, false, "", nonRelatedFstabEntry)
}

func (s *managedfsSuite) TestAttachFilesystemsExistingFstabEntry(c *gc.C) {
	existingFstabEntry := fmt.Sprintf("/dev/sda1 %s existing mtab stuff\n", testMountPoint)
	err := ioutil.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(existingFstabEntry), 0644)
	c.Assert(err, jc.ErrorIsNil)

	mtabEntry := fmt.Sprintf("/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, false, false, mtabEntry, existingFstabEntry)
}

func (s *managedfsSuite) TestAttachFilesystemsReadOnly(c *gc.C) {
	mtabEntry := fmt.Sprintf("\n/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, true, false, mtabEntry, mtabEntry+"\n")
}

func (s *managedfsSuite) TestAttachFilesystemsReattach(c *gc.C) {
	mtabEntry := fmt.Sprintf("/dev/sda1 %s other mtab stuff", testMountPoint)
	s.testAttachFilesystems(c, true, true, mtabEntry, "")
}

func (s *managedfsSuite) testAttachFilesystems(c *gc.C, readOnly, reattach bool, mtab, fstab string) {
	source := s.initSource(c)
	cmd := s.commands.expect("df", "--output=source", filepath.Dir(testMountPoint))
	cmd.respond("headers\n/same/as/rootfs", nil)
	cmd = s.commands.expect("df", "--output=source", testMountPoint)

	if mtab != "" {
		err := ioutil.WriteFile(filepath.Join(s.fakeEtcDir, "mtab"), []byte(mtab), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}

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

	results, err := source.AttachFilesystems(s.callCtx, []storage.FilesystemAttachmentParams{{
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
	c.Assert(results, jc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			names.NewFilesystemTag("0/0"),
			names.NewMachineTag("0"),
			storage.FilesystemAttachmentInfo{
				Path:     testMountPoint,
				ReadOnly: readOnly,
			},
		},
	}})

	if fstab != "" {
		data, err := ioutil.ReadFile(filepath.Join(s.fakeEtcDir, "fstab"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, fstab)
	}
}

func (s *managedfsSuite) TestDetachFilesystems(c *gc.C) {
	nonRelatedFstabEntry := "/dev/foo /mount/point stuff\n"
	fstabEntry := fmt.Sprintf("%s %s other mtab stuff", "/dev/sda1", testMountPoint)
	err := ioutil.WriteFile(filepath.Join(s.fakeEtcDir, "fstab"), []byte(nonRelatedFstabEntry+fstabEntry), 0644)
	c.Assert(err, jc.ErrorIsNil)
	source := s.initSource(c)
	testDetachFilesystems(c, s.commands, source, s.callCtx, true, s.fakeEtcDir, nonRelatedFstabEntry)
}

func (s *managedfsSuite) TestDetachFilesystemsUnattached(c *gc.C) {
	source := s.initSource(c)
	testDetachFilesystems(c, s.commands, source, s.callCtx, false, s.fakeEtcDir, "")
}
