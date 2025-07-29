// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"errors"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
)

func TestRootfsSuite(t *stdtesting.T) {
	tc.Run(t, &rootfsSuite{})
}

type rootfsSuite struct {
	testing.BaseSuite
	storageDir   string
	commands     *mockRunCommand
	mockDirFuncs *provider.MockDirFuncs
	fakeEtcDir   string
}

func (s *rootfsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageDir = c.MkDir()
	s.fakeEtcDir = c.MkDir()
}

func (s *rootfsSuite) TearDownTest(c *tc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *rootfsSuite) rootfsProvider(c *tc.C) storage.Provider {
	s.commands = &mockRunCommand{c: c}
	return provider.RootfsProvider(s.commands.run)
}

func (s *rootfsSuite) TestFilesystemSource(c *tc.C) {
	p := s.rootfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = p.FilesystemSource(cfg)
	c.Assert(err, tc.ErrorMatches, "storage directory not specified")
	cfg, err = storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{
		"storage-dir": c.MkDir(),
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = p.FilesystemSource(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *rootfsSuite) TestValidateConfig(c *tc.C) {
	p := s.rootfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.RootfsProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	// The rootfs provider does not have any user
	// configuration, so an empty map will pass.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *rootfsSuite) TestSupports(c *tc.C) {
	p := s.rootfsProvider(c)
	c.Assert(p.Supports(storage.StorageKindBlock), tc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), tc.IsTrue)
}

func (s *rootfsSuite) TestScope(c *tc.C) {
	p := s.rootfsProvider(c)
	c.Assert(p.Scope(), tc.Equals, storage.ScopeMachine)
}

func (s *rootfsSuite) rootfsFilesystemSource(c *tc.C, fakeMountInfo ...string) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	source, d := provider.RootfsFilesystemSource(s.fakeEtcDir, s.storageDir, s.commands.run, fakeMountInfo...)
	s.mockDirFuncs = d
	return source
}

func (s *rootfsSuite) TestCreateFilesystems(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n2048", nil)
	cmd = s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n4096", nil)

	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}, {
		Tag:  names.NewFilesystemTag("7"),
		Size: 4,
	}})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, []storage.CreateFilesystemsResult{{
		Filesystem: &storage.Filesystem{
			Tag: names.NewFilesystemTag("6"),
			FilesystemInfo: storage.FilesystemInfo{
				ProviderId: "6",
				Size:       2,
			},
		},
	}, {
		Filesystem: &storage.Filesystem{
			Tag: names.NewFilesystemTag("7"),
			FilesystemInfo: storage.FilesystemInfo{
				ProviderId: "7",
				Size:       4,
			},
		},
	}})
}

func (s *rootfsSuite) TestCreateFilesystemsIsUse(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("666"), // magic; see mockDirFuncs
		Size: 1,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "\".*/666\" is not empty")
}

func (s *rootfsSuite) TestAttachFilesystemsPathNotDir(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "file",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, `path "file" must be a directory`)
}

func (s *rootfsSuite) TestCreateFilesystemsNotEnoughSpace(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("1K-blocks\n2048", nil)

	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 4,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "filesystem is not big enough \\(2M < 4M\\)")
}

func (s *rootfsSuite) TestCreateFilesystemsInvalidPath(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	cmd := s.commands.expect("df", "--output=size", s.storageDir)
	cmd.respond("", errors.New("error creating directory"))

	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "getting size: error creating directory")
}

func (s *rootfsSuite) TestAttachFilesystemsNoPathSpecified(c *tc.C) {
	source := s.rootfsFilesystemSource(c)
	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "filesystem mount point not specified")
}

func (s *rootfsSuite) TestAttachFilesystemsBind(c *tc.C) {
	source := s.rootfsFilesystemSource(c)

	cmd := s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", nil)

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv",
			},
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBound(c *tc.C) {
	// already bind-mounted storage-dir/6 to the target
	mountInfo := mountInfoLine(666, 0, filepath.Join(s.storageDir, "6"), "/srv", "/dev/sda1")
	source := s.rootfsFilesystemSource(c, mountInfo)

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv",
			},
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBoundViaParent(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 667, filepath.Join("/some/parent/path", s.storageDir, "6"), "/srv", "/dev/sda1")
	mountInfo2 := mountInfoLine(667, 668, "/some/parent/path", "/", "/dev/sda1")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2)

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv",
			},
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBoundViaMultipleParents(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 667, filepath.Join("/some/parent/path", s.storageDir, "6"), "/srv", "/dev/sda1")
	mountInfo2 := mountInfoLine(667, 668, "/some/parent/path", "/another/parent/path", "/dev/sda1")
	mountInfo3 := mountInfoLine(668, 669, "/another/parent/path", "/", "/dev/sda1")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2, mountInfo3)

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv",
			},
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBindFailsDifferentFS(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 0, "/somewhere", filepath.Join(s.storageDir, "6"), "/dev")
	mountInfo2 := mountInfoLine(667, 0, "/src/of/root", "/srv", "/proc")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2)

	cmd := s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", errors.New("mount --bind fails"))

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, `".*/6" \("/dev"\) and "/srv" \("/proc"\) are on different filesystems`)
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSEmptyDir(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 0, "/somewhere", filepath.Join(s.storageDir, "6"), "/dev")
	mountInfo2 := mountInfoLine(667, 0, "/src/of/root", "/srv", "/dev")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2)

	cmd := s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv")
	cmd.respond("", errors.New("mount --bind fails"))

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv",
			},
		},
	}})
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSNonEmptyDirUnclaimed(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 0, "/somewhere", filepath.Join(s.storageDir, "6"), "/dev")
	mountInfo2 := mountInfoLine(667, 0, "/src/of/root", "/srv/666", "/dev")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2)

	cmd := s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv/666")
	cmd.respond("", errors.New("mount --bind fails"))

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv/666",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, `"/srv/666" is not empty`)
}

func (s *rootfsSuite) TestAttachFilesystemsBindSameFSNonEmptyDirClaimed(c *tc.C) {
	mountInfo1 := mountInfoLine(666, 0, "/somewhere", filepath.Join(s.storageDir, "6"), "/dev")
	mountInfo2 := mountInfoLine(667, 0, "/src/of/root", "/srv/666", "/dev")
	source := s.rootfsFilesystemSource(c, mountInfo1, mountInfo2)

	cmd := s.commands.expect("mount", "--bind", filepath.Join(s.storageDir, "6"), "/srv/666")
	cmd.respond("", errors.New("mount --bind fails"))

	s.mockDirFuncs.Dirs.Add(filepath.Join(s.storageDir, "6", "juju-target-claimed"))

	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		ProviderId: "6",
		Path:       "/srv/666",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("6"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv/666",
			},
		},
	}})
}

func (s *rootfsSuite) TestDetachFilesystems(c *tc.C) {
	mountInfo := mountInfoLine(666, 0, "/src/of/root", testMountPoint, "/dev/sda1")
	source := s.rootfsFilesystemSource(c, mountInfo)
	testDetachFilesystems(c, s.commands, source, true, s.fakeEtcDir, "")
}

func (s *rootfsSuite) TestDetachFilesystemsUnattached(c *tc.C) {
	// The "unattached" case covers both idempotency, and
	// also the scenario where bind-mounting failed. In
	// either case, there is no attachment-specific filesystem
	// mount.
	source := s.rootfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, false, s.fakeEtcDir, "")
}
