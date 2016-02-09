// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

const testControllersYAML = `
controllers:
  local.aws-test:
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  local.mallards:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  local.mark-test-prodstack:
    servers: [vm-23532.prodstack.canonical.com, great.test.server.hostname.co.nz]
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
`
