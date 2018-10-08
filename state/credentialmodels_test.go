// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type CredentialModelsSuite struct {
	ConnSuite

	credentialTag names.CloudCredentialTag
	abcModelTag   names.ModelTag
}

var _ = gc.Suite(&CredentialModelsSuite{})

func (s *CredentialModelsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.credentialTag = s.createCloudCredential(c, "foobar")
	s.abcModelTag = s.addModel(c, "abcmodel", s.credentialTag)
}

func (s *CredentialModelsSuite) createCloudCredential(c *gc.C, credentialName string) names.CloudCredentialTag {
	// Cloud name is always "dummy" as deep within the testing infrastructure,
	// we create a testing controller on a cloud "dummy".
	// Test cloud "dummy" only allows credentials with an empty auth type.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", "dummy", s.Owner.Id(), credentialName))
	err := s.State.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, jc.ErrorIsNil)
	return tag
}

func (s *CredentialModelsSuite) addModel(c *gc.C, modelName string, tag names.CloudCredentialTag) names.ModelTag {
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
	defer st.Close()
	return names.NewModelTag(uuid.String())
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccess(c *gc.C) {
	out, err := s.State.CredentialModelsAndOwnerAccess(s.credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, []state.CredentialOwnerModelAccess{
		{ModelName: "abcmodel", OwnerAccess: permission.AdminAccess, ModelUUID: s.abcModelTag.Id()},
	})
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccessMany(c *gc.C) {
	// add another model with the same credential
	xyzModelTag := s.addModel(c, "xyzmodel", s.credentialTag)

	// add another model with a different credential - should not be in the output.
	anotherCredential := s.createCloudCredential(c, "another")
	s.addModel(c, "dontshow", anotherCredential)

	out, err := s.State.CredentialModelsAndOwnerAccess(s.credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.SameContents, []state.CredentialOwnerModelAccess{
		{ModelName: "abcmodel", OwnerAccess: permission.AdminAccess, ModelUUID: s.abcModelTag.Id()},
		{ModelName: "xyzmodel", OwnerAccess: permission.AdminAccess, ModelUUID: xyzModelTag.Id()},
	})
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccessNoModels(c *gc.C) {
	anotherCredential := s.createCloudCredential(c, "another")

	out, err := s.State.CredentialModelsAndOwnerAccess(anotherCredential)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(out, gc.HasLen, 0)
}

func (s *CredentialModelsSuite) TestCredentialModels(c *gc.C) {
	out, err := s.State.CredentialModels(s.credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, map[string]string{s.abcModelTag.Id(): "abcmodel"})
}

func (s *CredentialModelsSuite) TestCredentialModelsExcludesDeadModels(c *gc.C) {
	checkModels := func(expected ...string) {
		out, err := s.State.CredentialModels(s.credentialTag)
		c.Assert(err, jc.ErrorIsNil)

		var obtained []string
		for k := range out {
			obtained = append(obtained, k)
		}
		c.Assert(obtained, jc.SameContents, expected)
	}

	// Add another model with the same credential.
	xyzModelTag := s.addModel(c, "xyzmodel", s.credentialTag)
	checkModels(s.abcModelTag.Id(), xyzModelTag.Id())

	// Set one of the models to Dead.
	m, r, err := s.StatePool.GetModel(s.abcModelTag.Id())
	defer r.Release()

	err = m.SetDead()
	c.Assert(err, jc.ErrorIsNil)

	checkModels(xyzModelTag.Id())
}

func (s *CredentialModelsSuite) TestCredentialNoModels(c *gc.C) {
	anotherCredential := s.createCloudCredential(c, "another")

	out, err := s.State.CredentialModels(anotherCredential)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(out, gc.HasLen, 0)
}
