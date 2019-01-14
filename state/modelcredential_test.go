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
	statetesting "github.com/juju/juju/state/testing"
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

func (s *ModelCredentialSuite) TestValidateCloudCredentialWrongCloud(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, `validating credential "stratus/bob/foobar" for cloud "dummy": cloud "stratus" not valid`)
}

func (s *ModelCredentialSuite) TestValidateCloudCredentialWrongAuthType(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, `validating credential "dummy/bob/foobar" for cloud "dummy": supported auth-types \["empty"\], "access-key" not supported`)
}

func (s *ModelCredentialSuite) TestValidateCloudCredentialModel(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	cred := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCredentialSuite) TestSetCloudCredential(c *gc.C) {
	s.assertSetCloudCredential(c,
		names.NewCloudCredentialTag("dummy/bob/foobar"),
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialNoUpdate(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	m := s.assertSetCloudCredential(c,
		tag,
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	set, err := m.SetCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	// This should be false as cloud credential change was an no-op.
	c.Assert(set, jc.IsFalse)

	// Check credential is still set.
	credentialTag, credentialSet := m.CloudCredential()
	c.Assert(credentialTag, gc.DeepEquals, tag)
	c.Assert(credentialSet, jc.IsTrue)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialInvalidCredentialContent(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	credential := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.InvalidateCloudCredential(tag, "test")
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, gc.ErrorMatches, `credential "dummy/bob/foobar" not valid`)
	c.Assert(set, jc.IsFalse)

	credentialTag, credentialSet := m.CloudCredential()
	// Make sure no credential is set.
	c.Assert(credentialTag, gc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, jc.IsFalse)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialInvalidCredentialForModel(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	credential := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"access-key": "someverysecretaccesskey",
		"secret-key": "someverysercretplainkey",
	})
	err = s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, gc.ErrorMatches, `cloud "stratus" not valid`)
	c.Assert(set, jc.IsFalse)

	credentialTag, credentialSet := m.CloudCredential()
	// Make sure no credential is set.
	c.Assert(credentialTag, gc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, jc.IsFalse)
}

func (s *ModelCredentialSuite) TestWatchModelCredential(c *gc.C) {
	// Credential to use in this test.
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	credential := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, jc.ErrorIsNil)

	// Model with credential watcher for this test.
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	w := m.WatchModelCredential()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)

	// Initial event.
	wc.AssertOneChange()

	// Check the watcher reacts to credential reference changes.
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set, jc.IsTrue)
	wc.AssertOneChange()

	// Check the watcher does not react to other changes on this model.
	err = m.SetDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Check that changes on another model do not affect this watcher.
	st := s.addModel(c, "abcmodel", s.credentialTag)
	defer st.Close()
	anotherM, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	set, err = anotherM.SetCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set, jc.IsTrue)
	wc.AssertNoChange()
}

func (s *ModelCredentialSuite) assertSetCloudCredential(c *gc.C, tag names.CloudCredentialTag, credential cloud.Credential) *state.Model {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	credentialTag, credentialSet := m.CloudCredential()
	// Make sure no credential is set.
	c.Assert(credentialTag, gc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, jc.IsFalse)

	err = s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, jc.ErrorIsNil)

	set, err := m.SetCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set, jc.IsTrue)

	// Check credential is set.
	credentialTag, credentialSet = m.CloudCredential()
	c.Assert(credentialTag, gc.DeepEquals, tag)
	c.Assert(credentialSet, jc.IsTrue)
	return m
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
