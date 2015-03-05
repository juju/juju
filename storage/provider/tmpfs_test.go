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

var _ = gc.Suite(&tmpfsSuite{})

type tmpfsSuite struct {
	testing.BaseSuite
	storageDir string
	commands   *mockRunCommand
}

func (s *tmpfsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageDir = c.MkDir()
}

func (s *tmpfsSuite) TearDownTest(c *gc.C) {
	s.commands.assertDrained()
	s.BaseSuite.TearDownTest(c)
}

func (s *tmpfsSuite) tmpfsProvider(c *gc.C) storage.Provider {
	s.commands = &mockRunCommand{c: c}
	return provider.TmpfsProvider(s.commands.run)
}

func (s *tmpfsSuite) TestFilesystemSource(c *gc.C) {
	p := s.tmpfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.FilesystemSource(nil, cfg)
	c.Assert(err, gc.ErrorMatches, "storage directory not specified")
	cfg, err = storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{
		"storage-dir": c.MkDir(),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.FilesystemSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *tmpfsSuite) TestValidateConfig(c *gc.C) {
	p := s.tmpfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	// The tmpfs provider does not have any user
	// configuration, so an empty map will pass.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *tmpfsSuite) TestSupports(c *gc.C) {
	p := s.tmpfsProvider(c)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsTrue)
}

func (s *tmpfsSuite) tmpfsFilesystemSource(c *gc.C) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	return provider.TmpfsFilesystemSource(
		s.storageDir,
		s.commands.run,
	)
}

func (s *tmpfsSuite) TestCreateFilesystems(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	cmd := s.commands.expect("mount", "-t", "tmpfs", "none", "/mnt/bar", "-o", "size=2097152")
	cmd.respond("", nil)
	cmd = s.commands.expect("df", "--output=size", "/mnt/bar")
	cmd.respond("1K-blocks\n2048", nil)

	filesystems, filesystemAttachments, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
		Attachment: &storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				InstanceId: "instance-id",
			},
			Path: "/mnt/bar",
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	mountedDirs := provider.MountedDirs(source)
	c.Assert(mountedDirs.Size(), gc.Equals, 1)
	c.Assert(mountedDirs.Contains("/mnt/bar"), jc.IsTrue)
	c.Assert(filesystems, gc.HasLen, 1)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
	c.Assert(filesystems[0], gc.Equals, storage.Filesystem{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	})
	c.Assert(filesystemAttachments[0], gc.Equals, storage.FilesystemAttachment{
		Path:       "/mnt/bar",
		Filesystem: names.NewFilesystemTag("6"),
		Machine:    names.NewMachineTag("1"),
	})
}

func (s *tmpfsSuite) TestCreateFilesystemsIsUse(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{
		{
			Tag:  names.NewFilesystemTag("6"),
			Size: 1,
			Attachment: &storage.FilesystemAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					Machine:    names.NewMachineTag("1"),
					InstanceId: "instance-id1",
				},
				Path: "/mnt/notempty",
			},
		}, {
			Tag:  names.NewFilesystemTag("6"),
			Size: 2,
			Attachment: &storage.FilesystemAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					Machine:    names.NewMachineTag("2"),
					InstanceId: "instance-id2",
				},
				Path: "/mnt/notempty",
			},
		}})
	c.Assert(err, gc.ErrorMatches, ".* path must be empty")
}

func (s *tmpfsSuite) TestCreateFilesystemsPathNotDir(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
		Attachment: &storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				InstanceId: "instance-id",
			},
			Path: "file",
		},
	}})
	c.Assert(err, gc.ErrorMatches, `.* path "file" must be a directory`)
}

func (s *tmpfsSuite) TestCreateFilesystemsNotEnoughSpace(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	cmd := s.commands.expect("mount", "-t", "tmpfs", "none", "/var/lib/juju/storage/fs/foo", "-o", "size=4194304")
	cmd.respond("", nil)
	cmd = s.commands.expect("df", "--output=size", "/var/lib/juju/storage/fs/foo")
	cmd.respond("1K-blocks\n2048", nil)

	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 4,
		Attachment: &storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				InstanceId: "instance-id",
			},
			Path: "/var/lib/juju/storage/fs/foo",
		},
	}})
	c.Assert(err, gc.ErrorMatches, ".* filesystem is not big enough \\(2M < 4M\\)")
}

func (s *tmpfsSuite) TestCreateFilesystemsInvalidPath(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	cmd := s.commands.expect("mount", "-t", "tmpfs", "none", "/mnt/bad-dir", "-o", "size=2097152")
	cmd.respond("", nil)
	cmd = s.commands.expect("df", "--output=size", "/mnt/bad-dir")
	cmd.respond("", errors.New("error creating directory"))

	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
		Attachment: &storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				InstanceId: "instance-id",
			},
			Path: "/mnt/bad-dir",
		},
	}})
	c.Assert(err, gc.ErrorMatches, ".* error creating directory")
}

func (s *tmpfsSuite) TestCreateFilesystemsNoAttachment(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}})
	c.Assert(err, gc.ErrorMatches, ".* creating filesystem without machine attachment not supported")
}

func (s *tmpfsSuite) TestCreateFilesystemsNoPathSpecified(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, _, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
		Attachment: &storage.FilesystemAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				InstanceId: "instance-id",
			},
		},
	}})
	c.Assert(err, gc.ErrorMatches, ".* cannot create a filesystem mount without specifying a path")
}
