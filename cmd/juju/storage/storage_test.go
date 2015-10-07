// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"os"

	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/featureflag"
)

var expectedSubCommmandNames = []string{
	"add",
	"help",
	"list",
	"pool",
	"show",
	"volume",
}

type storageSuite struct {
	HelpStorageSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestHelp(c *gc.C) {
	enableStorageFeature()
	s.command = storage.NewSuperCommand().(*storage.Command)
	s.assertHelp(c, expectedSubCommmandNames)
}

func (s *storageSuite) TestDisabled(c *gc.C) {
	storageCommand := storage.NewSuperCommand()
	ctx := testing.Context(c)
	code := cmd.Main(storageCommand, ctx, nil)
	c.Assert(testing.Stderr(ctx), gc.Equals, `
Enable experimental storage support by setting JUJU_DEV_FEATURE_FLAGS=storage.

error: storage is disabled
`[1:])
	c.Assert(code, gc.Equals, 1)
}

func enableStorageFeature() {
	if err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.Storage); err != nil {
		panic(err)
	}
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}
