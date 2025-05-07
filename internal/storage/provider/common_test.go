// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type providerCommonSuite struct{}

var _ = tc.Suite(&providerCommonSuite{})

func (s *providerCommonSuite) TestCommonProvidersExported(c *tc.C) {
	registry := provider.CommonStorageProviders()
	var common []storage.ProviderType
	pTypes, err := registry.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	for _, pType := range pTypes {
		common = append(common, pType)
		p, err := registry.StorageProvider(pType)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(p, tc.NotNil)
	}
	c.Assert(common, tc.SameContents, []storage.ProviderType{
		provider.LoopProviderType,
		provider.RootfsProviderType,
		provider.TmpfsProviderType,
	})
}

// testDetachFilesystems is a test-case for detaching filesystems that use
// the common "maybeUnmount" method.
func testDetachFilesystems(
	c *tc.C, commands *mockRunCommand,
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0], tc.ErrorIsNil)

	data, err := os.ReadFile(filepath.Join(etcDir, "fstab"))
	if os.IsNotExist(err) {
		c.Assert(fstab, tc.Equals, "")
		return
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, fstab)
}

func (s *providerCommonSuite) TestAllowedContainerProvider(c *tc.C) {
	c.Assert(provider.AllowedContainerProvider(provider.LoopProviderType), tc.IsTrue)
	c.Assert(provider.AllowedContainerProvider(provider.RootfsProviderType), tc.IsTrue)
	c.Assert(provider.AllowedContainerProvider(provider.TmpfsProviderType), tc.IsTrue)
	c.Assert(provider.AllowedContainerProvider("somestorage"), tc.IsFalse)
}
