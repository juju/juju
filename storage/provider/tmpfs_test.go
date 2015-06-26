// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"errors"
	"runtime"

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
	if runtime.GOOS == "windows" {
		c.Skip("Tests relevant only on *nix systems")
	}
	s.BaseSuite.SetUpTest(c)
	s.storageDir = c.MkDir()
}

func (s *tmpfsSuite) TearDownTest(c *gc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
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

func (s *tmpfsSuite) TestScope(c *gc.C) {
	p := s.tmpfsProvider(c)
	c.Assert(p.Scope(), gc.Equals, storage.ScopeMachine)
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

	filesystems, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, jc.DeepEquals, []storage.Filesystem{{
		Tag: names.NewFilesystemTag("6"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "filesystem-6",
			Size:         2,
		},
	}})
}

func (s *tmpfsSuite) TestCreateFilesystemsHugePages(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)

	// Set page size to 16MiB.
	s.PatchValue(provider.Getpagesize, func() int { return 16 * 1024 * 1024 })

	filesystems, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 17,
	}, {
		Tag:  names.NewFilesystemTag("2"),
		Size: 16,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, jc.DeepEquals, []storage.Filesystem{{
		Tag: names.NewFilesystemTag("1"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "filesystem-1",
			Size:         32,
		},
	}, {
		Tag: names.NewFilesystemTag("2"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "filesystem-2",
			Size:         16,
		},
	}})
}

func (s *tmpfsSuite) TestCreateFilesystemsIsUse(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1,
	}, {
		Tag:  names.NewFilesystemTag("1"),
		Size: 2,
	}})
	c.Assert(err, gc.ErrorMatches, "creating filesystem: filesystem 1 already exists")
}

func (s *tmpfsSuite) TestAttachFilesystemsPathNotDir(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1,
	}})
	c.Assert(err, jc.ErrorIsNil)
	_, err = source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "file",
	}})
	c.Assert(err, gc.ErrorMatches, `.* path "file" must be a directory`)
}

func (s *tmpfsSuite) TestAttachFilesystemsAlreadyMounted(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=source", "exists")
	cmd.respond("header\nfilesystem-123", nil)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("123"),
		Size: 1,
	}})
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("123"),
		Path:       "exists",
	}})
	c.Assert(attachments, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("123"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path: "exists",
		},
	}})
}

func (s *tmpfsSuite) TestAttachFilesystemsMountReadOnly(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, jc.ErrorIsNil)

	cmd := s.commands.expect("df", "--output=source", "/var/lib/juju/storage/fs/foo")
	cmd.respond("header\nvalue", nil)
	s.commands.expect("mount", "-t", "tmpfs", "filesystem-1", "/var/lib/juju/storage/fs/foo", "-o", "size=1024m,ro")

	attachments, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "/var/lib/juju/storage/fs/foo",
		AttachmentParams: storage.AttachmentParams{
			Machine:  names.NewMachineTag("2"),
			ReadOnly: true,
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("1"),
		Machine:    names.NewMachineTag("2"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path:     "/var/lib/juju/storage/fs/foo",
			ReadOnly: true,
		},
	}})
}

func (s *tmpfsSuite) TestAttachFilesystemsMountFails(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, jc.ErrorIsNil)

	cmd := s.commands.expect("df", "--output=source", "/var/lib/juju/storage/fs/foo")
	cmd.respond("header\nvalue", nil)
	cmd = s.commands.expect("mount", "-t", "tmpfs", "filesystem-1", "/var/lib/juju/storage/fs/foo", "-o", "size=1024m")
	cmd.respond("", errors.New("mount failed"))

	_, err = source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "/var/lib/juju/storage/fs/foo",
	}})
	c.Assert(err, gc.ErrorMatches, "attaching filesystem 1: cannot mount tmpfs: mount failed")
}

func (s *tmpfsSuite) TestAttachFilesystemsNoPathSpecified(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, jc.ErrorIsNil)
	_, err = source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
	}})
	c.Assert(err, gc.ErrorMatches, "attaching filesystem 6: filesystem mount point not specified")
}

func (s *tmpfsSuite) TestAttachFilesystemsNoFilesystem(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		Path:       "/mnt",
	}})
	c.Assert(err, gc.ErrorMatches, "attaching filesystem 6: reading filesystem info from disk: open .*/6.info: no such file or directory")
}

func (s *tmpfsSuite) TestDetachFilesystems(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, true)
}

func (s *tmpfsSuite) TestDetachFilesystemsUnattached(c *gc.C) {
	source := s.tmpfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, false)
}
