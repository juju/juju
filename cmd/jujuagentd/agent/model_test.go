// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	caasconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
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

// stubAgentConf injects a configuration into a ModelCommand without
// touching disk.
type stubAgentConf struct {
	agentconf.AgentConf
	conf agent.ConfigSetterWriter
}

func (s *stubAgentConf) CurrentConfig() agent.Config {
	return s.conf
}

func newModelAgentConfig(c *tc.C) agent.ConfigSetterWriter {
	conf, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewModelTag(coretesting.ModelTag.Id()),
		UpgradedToVersion: jujuversion.Current,
		Password:          "old-sekrit",
		APIAddresses:      []string{"10.0.0.1:17070"},
		Controller:        coretesting.ControllerTag,
		Model:             coretesting.ModelTag,
		CACert:            "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)
	return conf
}

// TestCertMaterialWithoutControllerAgentInfo proves the model agent
// reports missing controller certificate material rather than inventing
// it when its configuration carries none.
func (*ModelSuite) TestCertMaterialWithoutControllerAgentInfo(c *tc.C) {
	cmd := &ModelCommand{
		AgentConf: &stubAgentConf{conf: newModelAgentConfig(c)},
	}
	_, err := cmd.CertMaterial()
	c.Check(err, tc.ErrorIs, jujuerrors.NotFound)
	c.Check(err, tc.ErrorMatches, "controller agent info not found")
}

// TestCertMaterialCarriesControllerMaterial proves the model agent
// builds its certificate material from the ControllerAgentInfo and
// CACert values delivered through agent.conf.
// TODO (ice): replace the agent.conf certificate delivery with
// controller-issued certificate material.
func (*ModelSuite) TestCertMaterialCarriesControllerMaterial(c *tc.C) {
	conf := newModelAgentConfig(c)
	conf.SetControllerAgentInfo(controller.ControllerAgentInfo{
		Cert:           "controller-cert",
		PrivateKey:     "controller-key",
		CAPrivateKey:   "ca-key",
		APIPort:        17070,
		SystemIdentity: "identity",
	})
	cmd := &ModelCommand{
		AgentConf: &stubAgentConf{conf: conf},
	}
	material, err := cmd.CertMaterial()
	c.Check(err, tc.ErrorIsNil)
	c.Check(material, tc.DeepEquals, apiservercertwatcher.CertMaterial{
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "controller-cert",
		ControllerPrivateKey: "controller-key",
	})
}
