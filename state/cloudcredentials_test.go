// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
)

type CloudCredentialsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudCredentialsSuite{})

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialNew(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag("stratus/bob@local/foobar")
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
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag("stratus/bob@local/foobar")
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, jc.ErrorIsNil)

	cred = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "bob's nephew",
		"password": "simple",
	})
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
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	tag := names.NewCloudCredentialTag("stratus/bob@local/foobar")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating cloud credentials: credential "stratus/bob@local/foobar" with auth-type "userpass" is not supported \(expected one of \["access-key"\]\)`)
}

func (s *CloudCredentialsSuite) TestCloudCredentialsEmpty(c *gc.C) {
	creds, err := s.State.CloudCredentials(names.NewUserTag("bob"), "dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, gc.HasLen, 0)
}

func (s *CloudCredentialsSuite) TestCloudCredentials(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)
	otherUser := s.Factory.MakeUser(c, nil).UserTag()

	tag1 := names.NewCloudCredentialTag("stratus/bob@local/bobcred1")
	cred1 := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	err = s.State.UpdateCloudCredential(tag1, cred1)
	c.Assert(err, jc.ErrorIsNil)

	tag2 := names.NewCloudCredentialTag("stratus/" + otherUser.Canonical() + "/foobar")
	tag3 := names.NewCloudCredentialTag("stratus/bob@local/bobcred2")
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

	creds, err := s.State.CloudCredentials(names.NewUserTag("bob"), "stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[names.CloudCredentialTag]cloud.Credential{
		tag1: cred1,
		tag3: cred2,
	})
}

func (s *CloudCredentialsSuite) TestRemoveCredentials(c *gc.C) {
	// Create it.
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	tag := names.NewCloudCredentialTag("stratus/bob@local/bobcred1")
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
