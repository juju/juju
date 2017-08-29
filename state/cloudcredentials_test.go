// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
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
	})
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	// The retrieved credentials have labels although cloud.NewCredential
	// doesn't have them, so add it to the expected value.
	cred.Label = "foobar"

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, cred)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsExisting(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
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

	// The retrieved credentials have labels although cloud.NewCredential
	// doesn't have them, so add it to the expected value.
	cred.Label = "foobar"

	out, err := s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, cred)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialInvalidAuthType(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating cloud credentials: credential "stratus/bob/foobar" with auth-type "userpass" is not supported \(expected one of \["access-key"\]\)`)
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
	})
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

	for _, userName := range []string{"bob", "bob"} {
		creds, err := s.State.CloudCredentials(names.NewUserTag(userName), "stratus")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
			tag1.Id(): cred1,
			tag3.Id(): cred2,
		})
	}
}

func (s *CloudCredentialsSuite) TestRemoveCredentials(c *gc.C) {
	// Create it.
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
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
	err = s.State.RemoveCloudCredential(tag, false)
	c.Assert(err, jc.ErrorIsNil)

	// Check it.
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudCredentialsSuite) TestRemoveCredentialsInUse(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/bobcred1")
	cred := cloud.NewCredential(cloud.EmptyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err := s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	s.Factory.MakeUser(c, &factory.UserParams{
		Name: "bob",
	})
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "test",
		Owner:           names.NewUserTag("bob"),
		CloudName:       "dummy",
		CloudCredential: tag,
	})
	defer st.Close()

	// Try to remove the cloud credential nicely. Can't because a model is using it.
	err = s.State.RemoveCloudCredential(tag, false)
	c.Assert(err, gc.ErrorMatches, "removing cloud credential: cannot remove cloud credential \"cloudcred-dummy_bob_bobcred1\", still in use by 1 models: refcount changed")

	// Check it. Still there.
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the model.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Removes nicely now.
	err = s.State.RemoveCloudCredential(tag, false)
	c.Assert(err, jc.ErrorIsNil)

	// Check it.
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudCredentialsSuite) TestRevokeCredentialsInUse(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/bobcred1")
	cred := cloud.NewCredential(cloud.EmptyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err := s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.CloudCredential(tag)
	c.Assert(err, jc.ErrorIsNil)

	s.Factory.MakeUser(c, &factory.UserParams{
		Name: "bob",
	})
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "test",
		Owner:           names.NewUserTag("bob"),
		CloudName:       "dummy",
		CloudCredential: tag,
	})
	defer st.Close()

	// Remove the cloud credential forcefully.
	err = s.State.RemoveCloudCredential(tag, true)
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
	err = s.State.RemoveCloudCredential(cred, false)
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
