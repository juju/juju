// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

func fakeConfig(c *gc.C, attrs ...testing.Attrs) *config.Config {
	cfg, err := testing.ModelConfig(c).Apply(fakeConfigAttrs(attrs...))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func fakeConfigAttrs(attrs ...testing.Attrs) testing.Attrs {
	merged := testing.FakeConfig().Merge(testing.Attrs{
		"type": "equinix",
		"uuid": "ba532cc4-bc84-11eb-8529-0242ac130003",
	})
	for _, attrs := range attrs {
		merged = merged.Merge(attrs)
	}
	return merged
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
		Type:       "equinix",
		Name:       "equinix",
		Region:     "am",
		Endpoint:   "juju",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"project-id": "proj-juju-test",
		"api-token":  "password1",
	})
}
