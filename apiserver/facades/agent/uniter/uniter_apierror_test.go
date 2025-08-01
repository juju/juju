// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/testing"
)

type uniterAPIErrorSuite struct {
	testing.ApiServerSuite
}

func TestUniterAPIErrorSuite(t *stdtesting.T) {
	tc.Run(t, &uniterAPIErrorSuite{})
}

func (s *uniterAPIErrorSuite) SetupTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err := domainServices.Credential().UpdateCloudCredential(c.Context(), testing.DefaultCredentialId, cred)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *uniterAPIErrorSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestGetStorageStateError
`)
}
