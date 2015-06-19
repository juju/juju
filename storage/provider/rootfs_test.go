// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"errors"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&rootfsSuite{})

type rootfsSuite struct {
	testing.BaseSuite
	storageDir   string
	commands     *mockRunCommand
	mockDirFuncs *provider.MockDirFuncs
}

func (s *rootfsSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Tests relevant only on *nix systems")
	}
	s.BaseSuite.SetUpTest(c)
	s.storageDir = c.MkDir()
}

func (s *rootfsSuite) TearDownTest(c *gc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *rootfsSuite) rootfsProvider(c *gc.C) storage.Provider {
	s.commands = &mockRunCommand{c: c}
	return provider.RootfsProvider(s.commands.run)
}

func (s *rootfsSuite) TestFilesystemSource(c *gc.C) {
	p := s.rootfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.FilesystemSource(nil, cfg)
	c.Assert(err, gc.ErrorMatches, "storage directory not specified")
	cfg, err = storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{
		"storage-dir": c.MkDir(),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.FilesystemSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rootfsSuite) TestValidateConfig(c *gc.C) {
	p := s.rootfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	// The rootfs provider does not have any user
	// configuration, so an empty map will pass.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rootfsSuite) TestSupports(c *gc.C) {
	p := s.rootfsProvider(c)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsTrue)
}

func (s *rootfsSuite) TestScope(c *gc.C) {
	p := s.rootfsProvider(c)
	c.Assert(p.Scope(), gc.Equals, storage.ScopeMachine)
}

func (s *rootfsSuite) rootfsFilesystemSource(c *gc.C) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	source, d := provider.RootfsFilesystemSource(s.storageDir, s.commands.run)
	s.mockDirFuncs = d
	return source
}

func (s *rootfsSuite) TestCreateFilesystems(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n2048", nil)
	cmd = s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n4096", nil)

	filesystems, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}, {
		Tag:  names.NewFilesystemTag("7"),
		Size: 4,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(filesystems, jc.DeepEquals, []storage.Filesystem{{
		Tag: names.NewFilesystemTag("6"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "6",
			Size:         2,
		},
	}, {
		Tag: names.NewFilesystemTag("7"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "7",
			Size:         4,
		},
	}})
}

func (s *rootfsSuite) TestCreateFilesystemsIsUse(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("666"), // magic; see mockDirFuncs
		Size: 1,
	}})
	c.Assert(err, gc.ErrorMatches, "creating filesystem: \".*/666\" is not empty")
}

func (s *rootfsSuite) TestAttachFilesystemsPathNotDir(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	_, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "file",
	}})
	c.Assert(err, gc.ErrorMatches, `.* path "file" must be a directory`)
}

func (s *rootfsSuite) TestCreateFilesystemsNotEnoughSpace(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n2048", nil)

	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 4,
	}})
	c.Assert(err, gc.ErrorMatches, ".* filesystem is not big enough \\(2M < 4M\\)")
}

func (s *rootfsSuite) TestCreateFilesystemsInvalidPath(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("", errors.New("error creating directory"))

	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}})
	c.Assert(err, gc.ErrorMatches, ".* error creating directory")
}

func (s *rootfsSuite) TestAttachFilesystemsNoPathSpecified(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	_, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
	}})
	c.Assert(err, gc.ErrorMatches, "attaching filesystem 6: filesystem mount point not specified")
}

func (s *rootfsSuite) TestAttachFilesystemsBind(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("df", "--output=source", "/srv")
	cmd.respond("headers\n/src/of/root", nil)

	cmd = s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", nil)

	info, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("6"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path: "/srv",
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBound(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	// already bind-mounted storage-dir/6 to the target
	cmd := s.commands.expect("df", "--output=source", "/srv")
	cmd.respond("headers\n"+filepath.Join(s.storageDir, "6"), nil)

	info, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("6"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path: "/srv",
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBindFailsDifferentFS(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("df", "--output=source", "/srv")
	cmd.respond("headers\n/src/of/root", nil)

	cmd = s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", errors.New("mount --bind fails"))

	cmd = s.commands.expect("df", "--output=target", filepath.Join(s.storageDir, "6"))
	cmd.respond("headers\n/dev", nil)

	cmd = s.commands.expect("df", "--output=target", "/srv")
	cmd.respond("headers\n/proc", nil)

	_, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv",
	}})
	c.Assert(err, gc.ErrorMatches, `attaching filesystem 6: ".*/6" \("/dev"\) and "/srv" \("/proc"\) are on different filesystems`)
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSEmptyDir(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("df", "--output=source", "/srv")
	cmd.respond("headers\n/src/of/root", nil)

	cmd = s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", errors.New("mount --bind fails"))

	cmd = s.commands.expect("df", "--output=target", filepath.Join(s.storageDir, "6"))
	cmd.respond("headers\n/dev", nil)

	cmd = s.commands.expect("df", "--output=target", "/srv")
	cmd.respond("headers\n/dev", nil)

	info, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("6"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path: "/srv",
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSNonEmptyDirUnclaimed(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("df", "--output=source", "/srv/666")
	cmd.respond("headers\n/src/of/root", nil)

	cmd = s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv/666")
	cmd.respond("", errors.New("mount --bind fails"))

	cmd = s.commands.expect("df", "--output=target", filepath.Join(s.storageDir, "6"))
	cmd.respond("headers\n/dev", nil)

	cmd = s.commands.expect("df", "--output=target", "/srv/666")
	cmd.respond("headers\n/dev", nil)

	_, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv/666",
	}})
	c.Assert(err, gc.ErrorMatches, `attaching filesystem 6: "/srv/666" is not empty`)
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSNonEmptyDirClaimed(c *gc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("df", "--output=source", "/srv/666")
	cmd.respond("headers\n/src/of/root", nil)

	cmd = s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv/666")
	cmd.respond("", errors.New("mount --bind fails"))

	cmd = s.commands.expect("df", "--output=target", filepath.Join(s.storageDir, "6"))
	cmd.respond("headers\n/dev", nil)

	cmd = s.commands.expect("df", "--output=target", "/srv/666")
	cmd.respond("headers\n/dev", nil)

	s.mockDirFuncs.Dirs.Add(filepath.Join(s.storageDir, "6", "juju-target-claimed"))

	info, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Path:         "/srv/666",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []storage.FilesystemAttachment{{
		Filesystem: names.NewFilesystemTag("6"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path: "/srv/666",
		},
	}})
}

func (s *rootfsSuite) TestDetachFilesystems(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, true)
}

func (s *rootfsSuite) TestDetachFilesystemsUnattached(c *gc.C) {
	// The "unattached" case covers both idempotency, and
	// also the scenario where bind-mounting failed. In
	// either case, there is no attachment-specific filesystem
	// mount.
	source := s.rootfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, false)
}
