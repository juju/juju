// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type credentialInvalidator func(ctx context.Context, reason environs.CredentialInvalidReason) error

func (c credentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c(ctx, reason)
}

type baseProviderSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	controllerUUID string

	credentialInvalidator credentialInvalidator
	invalidCredential     bool
}

func (s *baseProviderSuite) setupFakeTools(c *tc.C) {
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	toolsDir := filepath.Join(storageDir, "tools")
	s.PatchValue(&envtools.DefaultBaseURL, utils.MakeFileURL(toolsDir))
	s.UploadFakeToolsToDirectory(c, storageDir, "released")
}

func (s *baseProviderSuite) SetUpSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*tc.C) {
		restoreFinishBootstrap()
	})
}

func (s *baseProviderSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.credentialInvalidator = func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		s.invalidCredential = true
		return nil
	}
}

func (s *baseProviderSuite) TearDownTest(c *tc.C) {
	s.invalidCredential = false
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *baseProviderSuite) TearDownSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}
