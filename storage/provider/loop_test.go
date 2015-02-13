// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

const stubMachineId = "machine101"

var _ = gc.Suite(&loopSuite{})

type loopSuite struct {
	testing.BaseSuite
	storageDir string
	commands   *mockRunCommand
	source     storage.VolumeSource
}

func (s *loopSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.storageDir = c.MkDir()
	s.commands = &mockRunCommand{c: c}
	s.source = provider.LoopVolumeSource(
		s.storageDir,
		s.commands.run,
	)
}

func (s *loopSuite) TearDownTest(c *gc.C) {
	s.commands.assertDrained()
	s.BaseSuite.TearDownTest(c)
}

func (s *loopSuite) TestVolumeSource(c *gc.C) {
	p := provider.LoopProvider(s.commands.run)
	cfg, err := storage.NewConfig("name", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.VolumeSource(nil, cfg)
	c.Assert(err, gc.ErrorMatches, "storage directory not specified")
	cfg, err = storage.NewConfig("name", provider.LoopProviderType, map[string]interface{}{
		"storage-dir": c.MkDir(),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.VolumeSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loopSuite) TestValidateConfig(c *gc.C) {
	p := provider.LoopProvider(s.commands.run)
	cfg, err := storage.NewConfig("name", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	// The loop provider does not have any user
	// configuration, so an empty map will pass.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loopSuite) TestCreateVolumes(c *gc.C) {
	s.commands.expect("fallocate", "-l", "2MiB", filepath.Join(s.storageDir, "disk-0"))
	cmd := s.commands.expect("losetup", "-f", "--show", filepath.Join(s.storageDir, "disk-0"))
	cmd.respond("/dev/loop99", nil)

	volumes, volumeAttachments, err := s.source.CreateVolumes([]storage.VolumeParams{{
		Tag:  names.NewDiskTag("0"),
		Size: 2,
		Attachment: &storage.AttachmentParams{
			Machine:    names.NewMachineTag("1"),
			InstanceId: "instance-id",
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 1)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	c.Assert(volumes[0], gc.Equals, storage.Volume{
		Tag:      names.NewDiskTag("0"),
		VolumeId: "disk-0",
		Size:     2,
	})
	c.Assert(volumeAttachments[0], gc.Equals, storage.VolumeAttachment{
		Volume:     names.NewDiskTag("0"),
		Machine:    names.NewMachineTag("1"),
		DeviceName: "loop99",
	})
}

func (s *loopSuite) TestCreateVolumesNoAttachment(c *gc.C) {
	_, _, err := s.source.CreateVolumes([]storage.VolumeParams{{
		Tag:  names.NewDiskTag("0"),
		Size: 2,
	}})
	c.Assert(err, gc.ErrorMatches, "creating volume: creating loop device without machine attachment not supported")
}

func (s *loopSuite) TestDestroyVolumes(c *gc.C) {
	fileName := filepath.Join(s.storageDir, "disk-0")
	cmd := s.commands.expect("losetup", "-j", fileName)
	cmd.respond("/dev/loop0: foo\n/dev/loop1: bar", nil)
	s.commands.expect("losetup", "-d", "/dev/loop0")
	s.commands.expect("losetup", "-d", "/dev/loop1")

	err := ioutil.WriteFile(fileName, nil, 0644)
	c.Assert(err, jc.ErrorIsNil)

	err = s.source.DestroyVolumes([]string{"disk-0"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loopSuite) TestDestroyVolumesDetachFails(c *gc.C) {
	fileName := filepath.Join(s.storageDir, "disk-0")
	cmd := s.commands.expect("losetup", "-j", fileName)
	cmd.respond("/dev/loop0: foo\n/dev/loop1: bar", nil)
	cmd = s.commands.expect("losetup", "-d", "/dev/loop0")
	cmd.respond("", errors.New("oy"))

	err := s.source.DestroyVolumes([]string{"disk-0"})
	c.Assert(err, gc.ErrorMatches, `detaching loop device "loop0": oy`)
}

func (s *loopSuite) TestDestroyVolumesInvalidVolumeId(c *gc.C) {
	err := s.source.DestroyVolumes([]string{"../super/important/stuff"})
	c.Assert(err, gc.ErrorMatches, `invalid loop volume ID "\.\./super/important/stuff"`)
}

func (s *loopSuite) TestDescribeVolumes(c *gc.C) {
	_, err := s.source.DescribeVolumes([]string{"a", "b"})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *loopSuite) TestAttachVolumes(c *gc.C) {
	_, err := s.source.AttachVolumes(nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *loopSuite) TestDetachVolumes(c *gc.C) {
	err := s.source.DetachVolumes(nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}
