// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type providerCommonSuite struct{}

var _ = gc.Suite(&providerCommonSuite{})

func (s *providerCommonSuite) TestCommonProvidersExported(c *gc.C) {
	registry := provider.CommonStorageProviders()
	var common []storage.ProviderType
	for _, pType := range registry.StorageProviderTypes() {
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
func testDetachFilesystems(c *gc.C, commands *mockRunCommand, source storage.FilesystemSource, mounted bool) {
	const testMountPoint = "/in/the/place"

	cmd := commands.expect("df", "--output=source", filepath.Dir(testMountPoint))
	cmd.respond("headers\n/same/as/rootfs", nil)
	cmd = commands.expect("df", "--output=source", testMountPoint)
	if mounted {
		cmd.respond("headers\n/different/to/rootfs", nil)
		commands.expect("umount", testMountPoint)
	} else {
		cmd.respond("headers\n/same/as/rootfs", nil)
	}

	results, err := source.DetachFilesystems([]storage.FilesystemAttachmentParams{{
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
}
