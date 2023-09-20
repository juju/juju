// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	ctx "context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/credentialmanager"
	"github.com/juju/juju/juju/testing"
)

// This suite only exists because no user facing calls exercise
// invalidate credential calls enough to expose serialisation bugs.
// If/when we have commands that would expose this,
// we should drop this suite and write a new command-based one.

type CredentialManagerIntegrationSuite struct {
	testing.ApiServerSuite
	client *credentialmanager.Client
}

var _ = gc.Suite(&CredentialManagerIntegrationSuite{})

func (s *CredentialManagerIntegrationSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	userConn := s.OpenControllerModelAPI(c)
	s.client = credentialmanager.NewClient(userConn)
}

func (s *CredentialManagerIntegrationSuite) TearDownTest(c *gc.C) {
	s.client.Close()
	s.ApiServerSuite.TearDownTest(c)
}

func (s *CredentialManagerIntegrationSuite) TestInvalidateModelCredential(c *gc.C) {
	model := s.ControllerModel(c)
	tag, set := model.CloudCredentialTag()
	c.Assert(set, jc.IsTrue)

	credService := s.ServiceFactoryGetter.FactoryForModel(s.ControllerModelUUID()).Credential()
	credential, err := credService.CloudCredential(ctx.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credential.Invalid, jc.IsFalse)

	c.Assert(s.client.InvalidateModelCredential("no reason really"), jc.ErrorIsNil)
	credential, err = credService.CloudCredential(ctx.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credential.Invalid, jc.IsTrue)
}
