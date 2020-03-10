// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type CloudCredentialsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudCredentialsSuite{})

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialNew(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	expected := statetesting.CloudCredential(cloud.AccessKeyAuthType,
		map[string]string{"bar": "bar val", "foo": "foo val"},
	)
	expected.DocID = "stratus#bob#foobar"
	expected.Owner = "bob"
	expected.Cloud = "stratus"
	expected.Name = "foobar"
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *CloudCredentialsSuite) TestCreateInvalidCredential(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	// Setting of these properties should have no effect when creating a new credential.
	cred.Invalid = true
	cred.InvalidReason = "because am testing you"
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, "creating cloud credential: adding invalid credential not supported")
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsExisting(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	cred = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	cred.Revoked = true
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	expected := statetesting.CloudCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	expected.DocID = "stratus#bob#foobar"
	expected.Owner = "bob"
	expected.Cloud = "stratus"
	expected.Name = "foobar"
	expected.Revoked = true

	c.Assert(out, jc.DeepEquals, expected)
}

func assertCredentialCreated(c *gc.C, testSuite ConnSuite) (string, *state.User, names.CloudCredentialTag) {
	owner := testSuite.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
		Name:     "bob",
	})

	cloudName := "stratus"
	err := testSuite.State.AddCloud(cloud.Cloud{
		Name:      cloudName,
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "dummy-region", Endpoint: "endpoint"}},
	}, owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	tag := createCredential(c, testSuite, cloudName, owner.Name(), "foobar")
	return cloudName, owner, tag
}

func createCredential(c *gc.C, testSuite ConnSuite, cloudName, userName, credentialName string) names.CloudCredentialTag {
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%v/%v/%v", cloudName, userName, credentialName))
	err := testSuite.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	return tag
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

func assertModelSuspended(c *gc.C, testSuite ConnSuite) (names.CloudCredentialTag, string) {
	// 1. Create a credential
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, testSuite)

	// 2. Create model on the test cloud with test credential
	modelUUID := assertModelCreated(c, testSuite, cloudName, credentialTag, credentialOwner.Tag(), "model-for-cloud")

	// 3. update credential to be invalid and check model is suspended
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	cred.Invalid = true
	cred.InvalidReason = "because it is really really invalid"
	err := testSuite.State.UpdateCloudCredential(credentialTag, cred)
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, testSuite.StatePool, modelUUID, status.Suspended)

	return credentialTag, modelUUID
}

func assertModelStatus(c *gc.C, pool *state.StatePool, testModelUUID string, expectedStatus status.Status) {
	aModel, helper, err := pool.GetModel(testModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	modelStatus, err := aModel.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelStatus.Status, gc.DeepEquals, expectedStatus)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsTouchesCredentialModels(c *gc.C) {
	// This test checks that models are affected when their credential validity is changed...
	// 1. create a credential
	// 2. set a model to use it
	// 3. update credential to be invalid and check model is suspended
	// 4. update credential bar its validity, check no changes in model state
	// 5. mark credential as valid and check that model is unsuspended

	// 1.2.3.
	tag, testModelUUID := assertModelSuspended(c, s.ConnSuite)

	// 4.
	storedCred, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedCred.IsValid(), jc.IsFalse)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	cred.Revoked = true
	// all other credential attributes remain unchanged
	cred.Invalid = storedCred.Invalid
	cred.InvalidReason = storedCred.InvalidReason

	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, s.StatePool, testModelUUID, status.Suspended)

	// 5.
	cred.Invalid = !storedCred.Invalid
	cred.InvalidReason = ""
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	assertModelStatus(c, s.StatePool, testModelUUID, status.Available)
}

func (s *CloudCredentialsSuite) TestRemoveModelsCredential(c *gc.C) {
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)
	modelUUID := assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), "model-for-cloud")

	err := s.State.RemoveModelsCredential(credentialTag)
	c.Assert(err, jc.ErrorIsNil)

	aModel, helper, err := s.StatePool.GetModel(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	_, isSet := aModel.CloudCredentialTag()
	c.Assert(isSet, jc.IsFalse)
	_, isSet, err = aModel.CloudCredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSet, jc.IsFalse)
}

func (s *CloudCredentialsSuite) TestRemoveModelsCredentialConcurrentModelDelete(c *gc.C) {
	logger := loggo.GetLogger("juju.state")
	logger.SetLogLevel(loggo.TRACE)
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)
	modelUUID := assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), "model-for-cloud")

	deleteModel := func() {
		aModel, helper, err := s.StatePool.GetModel(modelUUID)
		c.Assert(err, jc.ErrorIsNil)
		defer helper.Release()
		err = aModel.SetDead()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(aModel.Refresh(), jc.ErrorIsNil)
		c.Assert(aModel.Life(), gc.Equals, state.Dead)
	}
	defer state.SetBeforeHooks(c, s.State, deleteModel).Check()

	err := s.State.RemoveModelsCredential(credentialTag)
	c.Assert(err, jc.ErrorIsNil)

	aModel, helper, err := s.StatePool.GetModel(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer helper.Release()
	_, isSet := aModel.CloudCredentialTag()
	// Since the model was marked 'dead' in the middle of 1st transaction attempt,
	// and 2nd attempt would not have picked it up, the model credential would not actually be cleared.
	c.Assert(isSet, jc.IsTrue)
	_, isSet, err = aModel.CloudCredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSet, jc.IsTrue)
	c.Assert(c.GetTestLog(), jc.Contains, "creating operations to remove models credential, attempt 1")
}

func (s *CloudCredentialsSuite) TestRemoveModelsCredentialNotUsed(c *gc.C) {
	_, _, credentialTag := assertCredentialCreated(c, s.ConnSuite)
	err := s.State.RemoveModelsCredential(credentialTag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CloudCredentialsSuite) assertCredentialInvalidated(c *gc.C, tag names.CloudCredentialTag) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	cred = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	cred.Invalid = true
	cred.InvalidReason = "because it is really really invalid"
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	expected := statetesting.CloudCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	expected.DocID = strings.Replace(tag.Id(), "/", "#", -1)
	expected.Owner = tag.Owner().Id()
	expected.Cloud = tag.Cloud().Id()
	expected.Name = tag.Name()
	expected.Invalid = true
	expected.InvalidReason = "because it is really really invalid"

	c.Assert(out, jc.DeepEquals, expected)
}

func (s *CloudCredentialsSuite) TestInvalidateCredential(c *gc.C) {
	s.assertCredentialInvalidated(c, names.NewCloudCredentialTag("stratus/bob/foobar"))
}

func (s *CloudCredentialsSuite) assertCredentialMarkedValid(c *gc.C, tag names.CloudCredentialTag, credential cloud.Credential) {
	err := s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.IsValid(), jc.IsTrue)
}

func (s *CloudCredentialsSuite) TestMarkInvalidCredentialAsValidExplicitly(c *gc.C) {
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	// This call will ensure that there is an invalid credential to test with.
	s.assertCredentialInvalidated(c, tag)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	cred.Invalid = false
	s.assertCredentialMarkedValid(c, tag, cred)
}

func (s *CloudCredentialsSuite) TestMarkInvalidCredentialAsValidImplicitly(c *gc.C) {
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	// This call will ensure that there is an invalid credential to test with.
	s.assertCredentialInvalidated(c, tag)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
	s.assertCredentialMarkedValid(c, tag, cred)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialInvalidAuthType(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating credential "stratus/bob/foobar" for cloud "stratus": supported auth-types \["access-key"\], "userpass" not supported`)
}

func (s *CloudCredentialsSuite) TestCloudCredentialsEmpty(c *gc.C) {
	creds, err := s.State.CloudCredentials(names.NewUserTag("bob"), "dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, gc.HasLen, 0)
}

func (s *CloudCredentialsSuite) TestCloudCredentials(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	otherUser := s.Factory.MakeUser(c, nil).UserTag()

	tag1 := names.NewCloudCredentialTag("stratus/bob/bobcred1")
	cred1 := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err = s.State.UpdateCloudCredential(tag1, cred1)
	c.Assert(err, jc.ErrorIsNil)

	tag2 := names.NewCloudCredentialTag("stratus/" + otherUser.Id() + "/foobar")
	tag3 := names.NewCloudCredentialTag("stratus/bob/bobcred2")
	cred2 := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"baz": "baz val",
		"qux": "qux val",
	})
	err = s.State.UpdateCloudCredential(tag2, cred2)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateCloudCredential(tag3, cred2)
	c.Assert(err, jc.ErrorIsNil)

	cred1.Label = "bobcred1"
	cred2.Label = "bobcred2"

	expected1 := statetesting.CloudCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	expected1.DocID = "stratus#bob#bobcred1"
	expected1.Owner = "bob"
	expected1.Cloud = "stratus"
	expected1.Name = "bobcred1"

	expected2 := statetesting.CloudCredential(cloud.AccessKeyAuthType, map[string]string{
		"baz": "baz val",
		"qux": "qux val",
	})
	expected2.DocID = "stratus#bob#bobcred2"
	expected2.Owner = "bob"
	expected2.Cloud = "stratus"
	expected2.Name = "bobcred2"

	for _, userName := range []string{"bob", "bob"} {
		creds, err := s.State.CloudCredentials(names.NewUserTag(userName), "stratus")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(creds, jc.DeepEquals, map[string]state.Credential{
			tag1.Id(): expected1,
			tag3.Id(): expected2,
		})
	}
}

func (s *CloudCredentialsSuite) TestRemoveCredentials(c *gc.C) {
	// Create it.
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	tag := names.NewCloudCredentialTag("stratus/bob/bobcred1")
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	// Remove it.
	err = s.State.RemoveCloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	// Check it.
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudCredentialsSuite) createCredentialWatcher(c *gc.C, st *state.State, cred names.CloudCredentialTag) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	w := st.WatchCredential(cred)
	s.AddCleanup(func(c *gc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, st, w)
}

func (s *CloudCredentialsSuite) TestWatchCredential(c *gc.C) {
	cred := names.NewCloudCredentialTag("dummy/fred/default")
	w, wc := s.createCredentialWatcher(c, s.State, cred)
	wc.AssertOneChange() // Initial event.

	// Create
	dummyCred := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(cred, dummyCred)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Revoke
	dummyCred.Revoked = true
	err = s.State.UpdateCloudCredential(cred, dummyCred)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Remove.
	err = s.State.RemoveCloudCredential(cred)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *CloudCredentialsSuite) TestWatchCredentialIgnoresOther(c *gc.C) {
	cred := names.NewCloudCredentialTag("dummy/fred/default")
	w, wc := s.createCredentialWatcher(c, s.State, cred)
	wc.AssertOneChange() // Initial event.

	anotherCred := names.NewCloudCredentialTag("dummy/mary/default")
	dummyCred := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(anotherCred, dummyCred)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *CloudCredentialsSuite) createCloudCredential(c *gc.C, cloudName, userName, credentialName string) (names.CloudCredentialTag, state.Credential) {
	authType := cloud.AccessKeyAuthType
	attributes := map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	}

	err := s.State.AddCloud(cloud.Cloud{
		Name:      cloudName,
		Type:      "low",
		AuthTypes: cloud.AuthTypes{authType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(authType, attributes)

	// Cloud credential tag to use when looking up this credential.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName))
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	// Credential data as stored in state.
	expected := state.Credential{}
	expected.DocID = fmt.Sprintf("%s#%s#%s", cloudName, userName, credentialName)
	expected.Owner = userName
	expected.Cloud = cloudName
	expected.Name = credentialName
	expected.AuthType = string(authType)
	expected.Attributes = attributes

	return tag, expected
}

func (s *CloudCredentialsSuite) TestAllCloudCredentialsNotFound(c *gc.C) {
	out, err := s.State.AllCloudCredentials(names.NewUserTag("bob"))
	c.Assert(err, gc.ErrorMatches, "cloud credentials for \"bob\" not found")
	c.Assert(out, gc.IsNil)
}

func (s *CloudCredentialsSuite) TestAllCloudCredentials(c *gc.C) {
	_, one := s.createCloudCredential(c, "cirrus", "bob", "foobar")
	_, two := s.createCloudCredential(c, "stratus", "bob", "foobar")

	// Added to make sure it is not returned.
	s.createCloudCredential(c, "cumulus", "mary", "foobar")

	out, err := s.State.AllCloudCredentials(names.NewUserTag("bob"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []state.Credential{one, two})
}

func (s *CloudCredentialsSuite) TestInvalidateCloudCredential(c *gc.C) {
	oneTag, one := s.createCloudCredential(c, "cirrus", "bob", "foobar")
	c.Assert(one.IsValid(), jc.IsTrue)

	reason := "testing, testing 1,2,3"
	err := s.State.InvalidateCloudCredential(oneTag, reason)
	c.Assert(err, jc.ErrorIsNil)

	updated, err := s.State.CloudCredential(oneTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updated.IsValid(), jc.IsFalse)
	c.Assert(updated.InvalidReason, gc.DeepEquals, reason)
}

func (s *CloudCredentialsSuite) TestInvalidateCloudCredentialNotFound(c *gc.C) {
	tag := names.NewCloudCredentialTag("cloud/user/credential")
	err := s.State.InvalidateCloudCredential(tag, "just does not matter")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
