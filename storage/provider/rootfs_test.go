// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"errors"

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
	storageDir string
	commands   *mockRunCommand
}

func (s *rootfsSuite) SetUpTest(c *gc.C) {
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
	return provider.RootfsFilesystemSource(
		s.storageDir,
		s.commands.run,
	)
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
		Tag:          names.NewFilesystemTag("6"),
		FilesystemId: "6",
		Size:         2,
	}, {
		Tag:          names.NewFilesystemTag("7"),
		FilesystemId: "7",
		Size:         4,
	}})
}

func (s *rootfsSuite) TestCreateFilesystemsIsUse(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	_, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("666"), // magic; see mockDirFuncs
		Size: 1,
	}})
	c.Assert(err, gc.ErrorMatches, ".* path must be empty")
}

func (s *rootfsSuite) TestAttachFilesystemsPathNotDir(c *gc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("findmnt", "file")
	cmd.respond("", errors.New("not mounted"))
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
