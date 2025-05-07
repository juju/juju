// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
)

var _ = tc.Suite(&tmpfsSuite{})

type tmpfsSuite struct {
	testing.BaseSuite
	storageDir string
	commands   *mockRunCommand
	fakeEtcDir string
}

func mountInfoTmpfsLine(id int, mountPoint, source string) string {
	return fmt.Sprintf("%d 6666 8:1 / %s rw,relatime shared:1 - tmpfs %s rw,size=5120k,uid=1000000,gid=1000000,inode64", id, mountPoint, source)
}

func (s *tmpfsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageDir = c.MkDir()
	s.fakeEtcDir = c.MkDir()
}

func (s *tmpfsSuite) TearDownTest(c *tc.C) {
	if s.commands != nil {
		s.commands.assertDrained()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *tmpfsSuite) tmpfsProvider(c *tc.C) storage.Provider {
	s.commands = &mockRunCommand{c: c}
	return provider.TmpfsProvider(s.commands.run)
}

func (s *tmpfsSuite) TestFilesystemSource(c *tc.C) {
	p := s.tmpfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = p.FilesystemSource(cfg)
	c.Assert(err, tc.ErrorMatches, "storage directory not specified")
	cfg, err = storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{
		"storage-dir": c.MkDir(),
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = p.FilesystemSource(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *tmpfsSuite) TestValidateConfig(c *tc.C) {
	p := s.tmpfsProvider(c)
	cfg, err := storage.NewConfig("name", provider.TmpfsProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	// The tmpfs provider does not have any user
	// configuration, so an empty map will pass.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *tmpfsSuite) TestSupports(c *tc.C) {
	p := s.tmpfsProvider(c)
	c.Assert(p.Supports(storage.StorageKindBlock), tc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), tc.IsTrue)
}

func (s *tmpfsSuite) TestScope(c *tc.C) {
	p := s.tmpfsProvider(c)
	c.Assert(p.Scope(), tc.Equals, storage.ScopeMachine)
}

func (s *tmpfsSuite) tmpfsFilesystemSource(c *tc.C, fakeMountInfo ...string) storage.FilesystemSource {
	s.commands = &mockRunCommand{c: c}
	return provider.TmpfsFilesystemSource(
		s.fakeEtcDir,
		s.storageDir,
		s.commands.run,
		fakeMountInfo...,
	)
}

func (s *tmpfsSuite) TestCreateFilesystems(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)

	results, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("6"),
		Size: 2,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.CreateFilesystemsResult{{
		Filesystem: &storage.Filesystem{
			Tag: names.NewFilesystemTag("6"),
			FilesystemInfo: storage.FilesystemInfo{
				FilesystemId: "filesystem-6",
				Size:         2,
			},
		},
	}})
}

func (s *tmpfsSuite) TestCreateFilesystemsHugePages(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)

	// Set page size to 16MiB.
	s.PatchValue(provider.Getpagesize, func() int { return 16 * 1024 * 1024 })

	results, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 17,
	}, {
		Tag:  names.NewFilesystemTag("2"),
		Size: 16,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.CreateFilesystemsResult{{
		Filesystem: &storage.Filesystem{
			Tag: names.NewFilesystemTag("1"),
			FilesystemInfo: storage.FilesystemInfo{
				FilesystemId: "filesystem-1",
				Size:         32,
			},
		},
	}, {
		Filesystem: &storage.Filesystem{
			Tag: names.NewFilesystemTag("2"),
			FilesystemInfo: storage.FilesystemInfo{
				FilesystemId: "filesystem-2",
				Size:         16,
			},
		},
	}})
}

func (s *tmpfsSuite) TestCreateFilesystemsIsUse(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	results, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1,
	}, {
		Tag:  names.NewFilesystemTag("1"),
		Size: 2,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Assert(results[0].Error, tc.ErrorIsNil)
	c.Assert(results[1].Error, tc.ErrorMatches, "filesystem 1 already exists")
}

func (s *tmpfsSuite) TestAttachFilesystemsPathNotDir(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1,
	}})
	c.Assert(err, tc.ErrorIsNil)
	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "file",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, `path "file" must be a directory`)
}

func (s *tmpfsSuite) TestAttachFilesystemsAlreadyMounted(c *tc.C) {
	mountInfo := mountInfoTmpfsLine(666, testMountPoint, names.NewFilesystemTag("123").String())
	mountInfo2 := mountInfoTmpfsLine(667, "/some/mount/point", names.NewFilesystemTag("666").String())
	source := s.tmpfsFilesystemSource(c, mountInfo, mountInfo2)
	_, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("123"),
		Size: 1,
	}})
	c.Assert(err, tc.ErrorIsNil)
	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("123"),
		Path:       testMountPoint,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("123"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: testMountPoint,
			},
		},
	}})
}

func (s *tmpfsSuite) TestAttachFilesystemsMountReadOnly(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, tc.ErrorIsNil)

	s.commands.expect("mount", "-t", "tmpfs", "filesystem-1", "/var/lib/juju/storage/fs/foo", "-o", "size=1024m,ro")

	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "/var/lib/juju/storage/fs/foo",
		AttachmentParams: storage.AttachmentParams{
			Machine:  names.NewMachineTag("2"),
			ReadOnly: true,
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("1"),
			Machine:    names.NewMachineTag("2"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path:     "/var/lib/juju/storage/fs/foo",
				ReadOnly: true,
			},
		},
	}})
}

func (s *tmpfsSuite) TestAttachFilesystemsMountFails(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, tc.ErrorIsNil)

	cmd := s.commands.expect("mount", "-t", "tmpfs", "filesystem-1", "/var/lib/juju/storage/fs/foo", "-o", "size=1024m")
	cmd.respond("", errors.New("mount failed"))

	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("1"),
		Path:       "/var/lib/juju/storage/fs/foo",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "cannot mount tmpfs: mount failed")
}

func (s *tmpfsSuite) TestAttachFilesystemsNoPathSpecified(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	_, err := source.CreateFilesystems(context.Background(), []storage.FilesystemParams{{
		Tag:  names.NewFilesystemTag("1"),
		Size: 1024,
	}})
	c.Assert(err, tc.ErrorIsNil)
	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "filesystem mount point not specified")
}

func (s *tmpfsSuite) TestAttachFilesystemsNoFilesystem(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	results, err := source.AttachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem: names.NewFilesystemTag("6"),
		Path:       "/mnt",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0].Error, tc.ErrorMatches, "reading filesystem info from disk: open .*/6.info: no such file or directory")
}

func (s *tmpfsSuite) TestDetachFilesystems(c *tc.C) {
	mountInfo := mountInfoTmpfsLine(666, testMountPoint, names.NewFilesystemTag("0/0").String())
	source := s.tmpfsFilesystemSource(c, mountInfo)
	testDetachFilesystems(c, s.commands, source, true, s.fakeEtcDir, "")
}

func (s *tmpfsSuite) TestDetachFilesystemsUnattached(c *tc.C) {
	source := s.tmpfsFilesystemSource(c)
	testDetachFilesystems(c, s.commands, source, false, s.fakeEtcDir, "")
}
