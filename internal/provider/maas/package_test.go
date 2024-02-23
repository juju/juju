// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/juju/os/v2/series"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/environs/envcontext"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseProviderSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	controllerUUID string

	callCtx           envcontext.ProviderCallContext
	invalidCredential bool
}

func (s *baseProviderSuite) setupFakeTools(c *gc.C) {
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	toolsDir := filepath.Join(storageDir, "tools")
	s.PatchValue(&envtools.DefaultBaseURL, utils.MakeFileURL(toolsDir))
	s.UploadFakeToolsToDirectory(c, storageDir, "released", "released")
}

func (s *baseProviderSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) {
		restoreFinishBootstrap()
	})
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&series.HostSeries, func() (string, error) { return version.DefaultSupportedLTS(), nil })
	s.callCtx = envcontext.WithCredentialInvalidator(context.Background(), func(context.Context, string) error {
		s.invalidCredential = true
		return nil
	})
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.invalidCredential = false
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *baseProviderSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}
