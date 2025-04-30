// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type providerCommonSuite struct{}

var _ = gc.Suite(&providerCommonSuite{})

func (s *providerCommonSuite) TestCommonProvidersExported(c *gc.C) {
	registry := provider.CommonStorageProviders()
	var common []storage.ProviderType
	pTypes, err := registry.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	for _, pType := range pTypes {
		common = append(common, pType)
		p, err := registry.StorageProvider(pType)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p, gc.NotNil)
	}
	c.Assert(common, jc.SameContents, []storage.ProviderType{
		provider.LoopProviderType,
		provider.RootfsProviderType,
		provider.TmpfsProviderType,
	})
}

// testDetachFilesystems is a test-case for detaching filesystems that use
// the common "maybeUnmount" method.
func testDetachFilesystems(
	c *gc.C, commands *mockRunCommand,
	source storage.FilesystemSource,
	mounted bool,
	etcDir, fstab string,
) {
	if mounted {
		commands.expect("umount", testMountPoint)
	}

	results, err := source.DetachFilesystems(context.Background(), []storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("0/0"),
		FilesystemId: "filesystem-0-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: "inst-id",
		},
		Path: testMountPoint,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)

	data, err := os.ReadFile(filepath.Join(etcDir, "fstab"))
	if os.IsNotExist(err) {
		c.Assert(fstab, gc.Equals, "")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, fstab)
}

func (s *providerCommonSuite) TestAllowedContainerProvider(c *gc.C) {
	c.Assert(provider.AllowedContainerProvider(provider.LoopProviderType), jc.IsTrue)
	c.Assert(provider.AllowedContainerProvider(provider.RootfsProviderType), jc.IsTrue)
	c.Assert(provider.AllowedContainerProvider(provider.TmpfsProviderType), jc.IsTrue)
	c.Assert(provider.AllowedContainerProvider("somestorage"), jc.IsFalse)
}
