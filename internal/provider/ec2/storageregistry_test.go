// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/storage"
	internaltesting "github.com/juju/juju/internal/testing"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the ec2 environ.
type storageRegistrySuite struct{}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, storageRegistrySuite{})
}

// TestRecommendedPoolForKind ensures that EBS is the recommended storage pool
// for block and filesystem.
func (storageRegistrySuite) TestRecommendedPoolForKind(c *tc.C) {
	provider, err := environs.Provider("ec2")
	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "x",
		},
	)
	srv := localServer{}
	srv.startServer(c)
	defer srv.stopServer(c)

	modelConfig, err := config.New(config.NoDefaults, internaltesting.FakeConfig().Merge(
		internaltesting.Attrs{"type": "ec2"},
	))
	c.Assert(err, tc.ErrorIsNil)

	env, err := environs.Open(c.Context(), provider, environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type:       "ec2",
			Name:       "ec2test",
			Region:     *srv.region.RegionName,
			Endpoint:   *srv.region.Endpoint,
			Credential: &credential,
		},
		Config: modelConfig,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	p := env.RecommendedPoolForKind(storage.StorageKindBlock)
	c.Check(p.Name(), tc.Equals, "ebs")
	c.Check(p.Provider().String(), tc.Equals, "ebs")

	p = env.RecommendedPoolForKind(storage.StorageKindFilesystem)
	c.Check(p.Name(), tc.Equals, "ebs")
	c.Check(p.Provider().String(), tc.Equals, "ebs")
}
