// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/credentialmanager"
	"github.com/juju/juju/juju/testing"
)

// This suite only exists because no user facing calls exercise
// invalidate credential calls enough to expose serialisation bugs.
// If/when we have commands that would expose this,
// we should drop this suite and write a new command-based one.

type CredentialManagerSuite struct {
	testing.JujuConnSuite
	client *credentialmanager.Client
}

func (s *CredentialManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	info := s.APIInfo(c)
	userConn := s.OpenAPIAs(c, info.Tag, info.Password)

	s.client = credentialmanager.NewClient(userConn)
}

func (s *CredentialManagerSuite) TearDownTest(c *gc.C) {
	s.client.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *gc.C) {
	tag, set := s.Model.CloudCredentialTag()
	c.Assert(set, jc.IsTrue)
	credential, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credential.IsValid(), jc.IsTrue)

	c.Assert(s.client.InvalidateModelCredential("no reason really"), jc.ErrorIsNil)

	credential, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credential.IsValid(), jc.IsFalse)
}
