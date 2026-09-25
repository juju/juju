// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	caasconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	coretesting "github.com/juju/juju/internal/testing"
)

type ModelSuite struct{}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &ModelSuite{})
}

func (*ModelSuite) TestNewContainerBrokerUnsupportedCloudType(c *tc.C) {
	_, err := newContainerBroker(c.Context(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{Type: "lxd"},
	}, nil)
	c.Check(err, tc.ErrorIs, jujuerrors.NotSupported)
	c.Check(err, tc.ErrorMatches, `cloud type "lxd" not supported`)
}

func (*ModelSuite) TestNewContainerBrokerOpensKubernetesProvider(c *tc.C) {
	// An incomplete spec still reaches the kubernetes provider, whose
	// cloud-spec validation reports the failure. A registry lookup would
	// fail differently, so this pins the direct-construction path.
	_, err := newContainerBroker(c.Context(), environs.OpenParams{
		Cloud:  environscloudspec.CloudSpec{Type: caasconstants.CAASProviderType},
		Config: coretesting.ModelConfig(c),
	}, nil)
	c.Check(err, tc.ErrorMatches, `validating cloud spec: cloud name "" not valid`)
}
