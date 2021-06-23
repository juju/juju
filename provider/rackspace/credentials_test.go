// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/provider/rackspace"
)

var _ = gc.Suite(&CredentialSuite{})

type CredentialSuite struct {
	testing.IsolationSuite
}

func (CredentialSuite) TestCredentialSchemasNoDomain(c *gc.C) {
	schemas := rackspace.Credentials{}.CredentialSchemas()
	for name, schema := range schemas {
		for _, attr := range schema {
			if attr.Name == openstack.CredAttrDomainName {
				c.Fatalf("schema %q has domain name attribute", name)
			}
		}
	}
}

func (CredentialSuite) TestDetectCredentialsNoDomain(c *gc.C) {
	os.Setenv("OS_USERNAME", "foo")
	os.Setenv("OS_TENANT_NAME", "baz")
	os.Setenv("OS_PASSWORD", "bar")
	os.Setenv("OS_DOMAIN_NAME", "domain")
	result, err := rackspace.Credentials{}.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	for _, v := range result.AuthCredentials {
		attr := v.Attributes()
		if _, ok := attr[openstack.CredAttrDomainName]; ok {
			c.Fatal("Domain name exists in rackspace creds and should not.")
		}
		c.Assert(v.Label, gc.Not(gc.Equals), "")
	}
}
