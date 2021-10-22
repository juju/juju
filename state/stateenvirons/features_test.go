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
	coretesting "github.com/juju/juju/testing"
)

type featuresSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&featuresSuite{})

func (s *featuresSuite) TestSupportedFeaturesWithIncompatibleEnviron(c *gc.C) {
	defer func(getter func(environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error)) {
		environGetter = getter
	}(environGetter)
	environGetter = func(environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error) {
		// Not supporting environs.SupportedFeaturesEnumerator
		return nil, nil
	}

	jujuVersion := version.MustParse("2.9.17")

	fs, err := SupportedFeatures(mockModel{jujuVersion: jujuVersion}, nil)
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

func (s *featuresSuite) TestSupportedFeaturesWithCompatibleEnviron(c *gc.C) {
	defer func(getter func(environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error)) {
		environGetter = getter
	}(environGetter)
	environGetter = func(environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error) {
		return mockEnvironWithFeatures{}, nil
	}

	jujuVersion := version.MustParse("2.9.17")

	fs, err := SupportedFeatures(mockModel{jujuVersion: jujuVersion}, nil)
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

type mockModel struct {
	Model

	jujuVersion version.Number
}

func (m mockModel) Config() (*config.Config, error) {
	return config.New(config.NoDefaults,
		coretesting.FakeConfig().Merge(coretesting.Attrs{
			config.AgentVersionKey: m.jujuVersion.String(),
		}),
	)
}

type mockEnvironWithFeatures struct {
	environs.Environ
}

func (mockEnvironWithFeatures) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "web-scale"})
	return fs, nil
}
