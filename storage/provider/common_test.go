// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type providerCommonSuite struct{}

var _ = gc.Suite(&providerCommonSuite{})

func (s *providerCommonSuite) TestCommonProvidersExported(c *gc.C) {
	var common []storage.ProviderType
	for pType, p := range provider.CommonProviders() {
		common = append(common, pType)
		_, ok := p.(storage.Provider)
		c.Check(ok, jc.IsTrue)
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

	err := source.DetachFilesystems([]storage.FilesystemAttachmentParams{{
		Filesystem:   names.NewFilesystemTag("0/0"),
		FilesystemId: "filesystem-0-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: "inst-id",
		},
		Path: testMountPoint,
	}})
	c.Assert(err, jc.ErrorIsNil)
}
