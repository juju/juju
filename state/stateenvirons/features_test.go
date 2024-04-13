// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type featuresSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&featuresSuite{})

func (s *featuresSuite) TestSupportedFeaturesWithIncompatibleEnviron(c *gc.C) {
	defer func(getter func(Model, CloudService, CredentialService) (environs.Environ, error)) {
		iaasEnvironGetter = getter
	}(iaasEnvironGetter)
	iaasEnvironGetter = func(Model, CloudService, CredentialService) (environs.Environ, error) {
		// Not supporting environs.SupportedFeaturesEnumerator
		return nil, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeIAAS,
	}
	fs, err := SupportedFeatures(m, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
	}

	c.Assert(fs.AsList(), gc.DeepEquals, exp)
}

func (s *featuresSuite) TestSupportedFeaturesWithCompatibleIAASEnviron(c *gc.C) {
	defer func(getter func(Model, CloudService, CredentialService) (environs.Environ, error)) {
		iaasEnvironGetter = getter
	}(iaasEnvironGetter)
	iaasEnvironGetter = func(Model, CloudService, CredentialService) (environs.Environ, error) {
		return mockIAASEnvironWithFeatures{}, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeIAAS,
	}
	fs, err := SupportedFeatures(m, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
		// The following feature was reported by the environ.
		{Name: "web-scale"},
	}

	c.Assert(fs.AsList(), gc.DeepEquals, exp)
}

func (s *featuresSuite) TestSupportedFeaturesWithCompatibleCAASEnviron(c *gc.C) {
	defer func(getter func(Model, CloudService, CredentialService) (caas.Broker, error)) {
		caasBrokerGetter = getter
	}(caasBrokerGetter)
	caasBrokerGetter = func(Model, CloudService, CredentialService) (caas.Broker, error) {
		return mockCAASEnvironWithFeatures{}, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeCAAS,
	}
	fs, err := SupportedFeatures(m, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
		// The following feature was reported by the environ.
		{Name: "k8s-api"},
	}

	c.Assert(fs.AsList(), gc.DeepEquals, exp)
}

type mockModel struct {
	Model

	jujuVersion version.Number
	modelType   state.ModelType
}

func (m mockModel) Config() (*config.Config, error) {
	return config.New(config.NoDefaults,
		coretesting.FakeConfig().Merge(coretesting.Attrs{
			config.AgentVersionKey: m.jujuVersion.String(),
		}),
	)
}

func (m mockModel) Type() state.ModelType {
	return m.modelType
}

type mockIAASEnvironWithFeatures struct {
	environs.Environ
}

func (mockIAASEnvironWithFeatures) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "web-scale"})
	return fs, nil
}

type mockCAASEnvironWithFeatures struct {
	caas.Broker
}

func (mockCAASEnvironWithFeatures) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "k8s-api"})
	return fs, nil
}
