// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
	_, exists := m.CloudCredentialTag()
	c.Assert(exists, jc.IsFalse)

	reason := "special invalidation"
	err = s.State.InvalidateModelCredential(reason)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCredentialSuite) TestInvalidateModelCredential(c *gc.C) {
	st := s.addModel(c, "abcmodel", s.credentialTag)
	defer st.Close()

	reason := "special invalidation"
	err := st.InvalidateModelCredential(reason)
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	info, err := m.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, status.StatusInfo{
		Status:  "suspended",
		Message: "suspended since cloud credential is not valid",
		Data:    map[string]interface{}{"reason": "special invalidation"},
	})
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
	credentialTag, credentialSet := m.CloudCredentialTag()
	c.Assert(credentialTag, gc.DeepEquals, tag)
	c.Assert(credentialSet, jc.IsTrue)
}

func (s *ModelCredentialSuite) TestWatchModelCredential(c *gc.C) {
	// Credential to use in this test.
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")

	// Model with credential watcher for this test.
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	w := m.WatchModelCredential()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

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
	credentialTag, credentialSet := m.CloudCredentialTag()
	// Make sure no credential is set.
	c.Assert(credentialTag, gc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, jc.IsFalse)

	set, err := m.SetCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set, jc.IsTrue)

	// Check credential is set.
	credentialTag, credentialSet = m.CloudCredentialTag()
	c.Assert(credentialTag, gc.DeepEquals, tag)
	c.Assert(credentialSet, jc.IsTrue)
	return m
}

func (s *ModelCredentialSuite) createCloudCredential(c *gc.C, credentialName string) names.CloudCredentialTag {
	// Cloud name is always "dummy" as deep within the testing infrastructure,
	// we create a testing controller on a cloud "dummy".
	// Test cloud "dummy" only allows credentials with an empty auth type.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", "dummy", s.Owner.Id(), credentialName))
	return tag
}

func (s *ModelCredentialSuite) addModel(c *gc.C, modelName string, tag names.CloudCredentialTag) *state.State {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": modelName,
		"uuid": uuid.String(),
	})
	_, st, err := s.Controller.NewModel(state.NoopConfigSchemaSource, state.ModelArgs{
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

func assertCredentialCreated(c *gc.C, testSuite ConnSuite) (string, *state.User, names.CloudCredentialTag) {
	owner := testSuite.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
		Name:     "bob",
	})

	cloudName := "stratus"

	tag := names.NewCloudCredentialTag(fmt.Sprintf("%v/%v/%v", cloudName, owner.Name(), "foobar"))
	return cloudName, owner, tag
}

func assertModelCreated(c *gc.C, testSuite ConnSuite, cloudName string, credentialTag names.CloudCredentialTag, owner names.Tag, modelName string) string {
	// Test model needs to be on the test cloud for all validation to pass.
	modelState := testSuite.Factory.MakeModel(c, &factory.ModelParams{
		Name:            modelName,
		CloudCredential: credentialTag,
		Owner:           owner,
		CloudName:       cloudName,
	})
	defer modelState.Close()
	testModel, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, testSuite.StatePool, testModel.UUID(), status.Available)
	return testModel.UUID()
}

func assertModelStatus(c *gc.C, pool *state.StatePool, testModelUUID string, expectedStatus status.Status) {
	aModel, helper, err := pool.GetModel(testModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	modelStatus, err := aModel.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelStatus.Status, gc.DeepEquals, expectedStatus)
}

func (s *ModelCredentialSuite) TestInvalidateModelCredentialTouchesAllCredentialModels(c *gc.C) {
	// This test checks that all models are affected when one of them invalidates a credential they all use...

	// 1. create a credential
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)

	// 2. create some models to use it
	modelUUIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		modelUUIDs[i] = assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), fmt.Sprintf("model-for-cloud%v", i))
	}

	// 3. invalidate credential
	oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	c.Assert(oneModelState.State().InvalidateModelCredential("testing invalidate for all credential models"), jc.ErrorIsNil)

	// 4. check all models are suspended
	for _, uuid := range modelUUIDs {
		assertModelStatus(c, s.StatePool, uuid, status.Suspended)
		assertModelHistories(c, s.StatePool, uuid, status.Suspended, status.Available)
	}
}

func assertModelSuspended(c *gc.C, testSuite ConnSuite) (names.CloudCredentialTag, string) {
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, testSuite)
	modelUUID := assertModelCreated(c, testSuite, cloudName, credentialTag, credentialOwner.Tag(), "model-for-cloud")
	m, helper, err := testSuite.StatePool.GetModel(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	err = m.State().CloudCredentialUpdated(credentialTag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.State().InvalidateModelCredential("broken")
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, testSuite.StatePool, modelUUID, status.Suspended)

	return credentialTag, modelUUID
}

func (s *ModelCredentialSuite) TestCloudCredentialUpdatedTouchesCredentialModels(c *gc.C) {
	tag, testModelUUID := assertModelSuspended(c, s.ConnSuite)

	err := s.State.CloudCredentialUpdated(tag)
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, s.StatePool, testModelUUID, status.Available)
}

func (s *ModelCredentialSuite) TestSetCredentialRevertsModelStatus(c *gc.C) {
	// 1. create a credential
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)

	// 2. create some models to use it
	validModelStatuses := []status.Status{
		status.Available,
		status.Busy,
		status.Destroying,
		status.Error,
	}
	desiredNumber := len(validModelStatuses)

	modelUUIDs := make([]string, desiredNumber)
	for i := 0; i < desiredNumber; i++ {
		modelUUIDs[i] = assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), fmt.Sprintf("model-for-cloud%v", i))
		oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[i])
		c.Assert(err, jc.ErrorIsNil)
		defer helper.Release()
		if validModelStatuses[i] != status.Available {
			// any model would be in 'available' status on setup.
			err = oneModelState.SetStatus(status.StatusInfo{Status: validModelStatuses[i]}, status.NoopStatusHistoryRecorder)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(oneModelState.Refresh(), jc.ErrorIsNil)
		}
		if i == desiredNumber-1 {
			// 3. invalidate credential on last model
			c.Assert(oneModelState.State().InvalidateModelCredential("testing"), jc.ErrorIsNil)
		}
	}

	// 4. check model is suspended
	for i := 0; i < desiredNumber; i++ {
		assertModelStatus(c, s.StatePool, modelUUIDs[i], status.Suspended)
		if validModelStatuses[i] == status.Available {
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Suspended, status.Available)
		} else {
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Suspended, validModelStatuses[i], status.Available)
		}
	}

	// 5. create another credential on the same cloud
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
		Name:     "uncle",
	})
	anotherCredentialTag := names.NewCloudCredentialTag(fmt.Sprintf("%v/%v/%v", cloudName, owner.Name(), "barfoo"))

	for i := 0; i < desiredNumber; i++ {
		oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[i])
		c.Assert(err, jc.ErrorIsNil)
		defer helper.Release()

		isSet, err := oneModelState.SetCloudCredential(anotherCredentialTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(isSet, jc.IsTrue)

		// 5. Check model status is reverted
		if validModelStatuses[i] == status.Available {
			assertModelStatus(c, s.StatePool, modelUUIDs[i], status.Available)
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Available, status.Suspended, status.Available)
		} else {
			assertModelStatus(c, s.StatePool, modelUUIDs[i], validModelStatuses[i])
			assertModelHistories(c, s.StatePool, modelUUIDs[i], validModelStatuses[i], status.Suspended, validModelStatuses[i], status.Available)
		}
	}
}

func assertModelHistories(c *gc.C, pool *state.StatePool, testModelUUID string, expected ...status.Status) []status.StatusInfo {
	aModel, helper, err := pool.GetModel(testModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	statusHistories, err := aModel.StatusHistory(status.StatusHistoryFilter{Size: 100})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusHistories, gc.HasLen, len(expected))
	for i, one := range expected {
		c.Assert(statusHistories[i].Status, gc.Equals, one)
	}
	return statusHistories
}
