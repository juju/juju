// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
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
