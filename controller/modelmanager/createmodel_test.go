// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller/modelmanager"
	_ "github.com/juju/juju/provider/all"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type RestrictedProviderFieldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RestrictedProviderFieldsSuite{})

func (*RestrictedProviderFieldsSuite) TestRestrictedProviderFields(c *gc.C) {
	for i, test := range []struct {
		provider string
		expected []string
	}{{
		provider: "azure",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
			"subscription-id", "tenant-id", "application-id", "application-password",
			"location", "controller-resource-group", "storage-account-type",
		},
	}, {
		provider: "dummy",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
		},
	}, {
		provider: "joyent",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
		},
	}, {
		provider: "maas",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
			"maas-server",
		},
	}, {
		provider: "openstack",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
			"region", "auth-url", "auth-mode",
		},
	}, {
		provider: "ec2",
		expected: []string{
			"type", "ca-cert", "state-port", "api-port", "controller-uuid",
			"region",
		},
	}} {
		c.Logf("%d: %s provider", i, test.provider)
		fields, err := modelmanager.RestrictedProviderFields(test.provider)
		c.Check(err, jc.ErrorIsNil)
		c.Check(fields, jc.SameContents, test.expected)
	}
}
