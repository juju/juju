// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type ModelCredentialSuite struct {
	ConnSuite

	credentialTag names.CloudCredentialTag
}

var _ = gc.Suite(&ModelCredentialSuite{})

func (s *ModelCredentialSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.credentialTag = s.createCloudCredential(c, "foobar")
}

func (s *ModelCredentialSuite) TestInvalidateModelCredentialNone(c *gc.C) {
	// The model created in ConnSuite does not have a credential.
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, exists := m.CloudCredential()
	c.Assert(exists, jc.IsFalse)

	reason := "special invalidation"
	err = s.State.InvalidateModelCredential(reason)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCredentialSuite) TestInvalidateModelCredential(c *gc.C) {
	st := s.addModel(c, "abcmodel", s.credentialTag)
	defer st.Close()
	credential, err := s.State.CloudCredential(s.credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credential.IsValid(), jc.IsTrue)

	reason := "special invalidation"
	err = st.InvalidateModelCredential(reason)
	c.Assert(err, jc.ErrorIsNil)

	invalidated, err := s.State.CloudCredential(s.credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(invalidated.IsValid(), jc.IsFalse)
	c.Assert(invalidated.InvalidReason, gc.DeepEquals, reason)
}

func (s *ModelCredentialSuite) createCloudCredential(c *gc.C, credentialName string) names.CloudCredentialTag {
	// Cloud name is always "dummy" as deep within the testing infrastructure,
	// we create a testing controller on a cloud "dummy".
	// Test cloud "dummy" only allows credentials with an empty auth type.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", "dummy", s.Owner.Id(), credentialName))
	err := s.State.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, jc.ErrorIsNil)
	return tag
}

func (s *ModelCredentialSuite) addModel(c *gc.C, modelName string, tag names.CloudCredentialTag) *state.State {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": modelName,
		"uuid": uuid.String(),
	})
	_, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   tag.Owner(),
		CloudCredential:         tag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}
