// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
)

type CloudCredentialsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudCredentialsSuite{})

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsNew(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	creds := map[string]cloud.Credential{
		"cred1": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		}),
		"cred2": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"a": "a val",
			"b": "b val",
		}),
		"cred3": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"user":     "bob",
			"password": "bob's password",
		}),
	}
	addCredLabels(creds)

	err = s.State.UpdateCloudCredentials(names.NewUserTag("bob"), "stratus", creds)
	c.Assert(err, jc.ErrorIsNil)
	// The retrieved credentials have labels although cloud.NewCredential
	// doesn't have them, so add them.
	for name, cred := range creds {
		cred.Label = name
		creds[name] = cred
	}
	creds1, err := s.State.CloudCredentials(names.NewUserTag("bob"), "stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds1, jc.DeepEquals, creds)
}

func (s *CloudCredentialsSuite) TestCloudCredentialsEmpty(c *gc.C) {
	creds, err := s.State.CloudCredentials(names.NewUserTag("bob"), "dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, gc.HasLen, 0)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsExisting(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateCloudCredentials(names.NewUserTag("bob"), "stratus", map[string]cloud.Credential{
		"cred1": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		}),
		"cred2": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"a": "a val",
			"b": "b val",
		}),
		"cred3": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"user":     "bob",
			"password": "bob's password",
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateCloudCredentials(names.NewUserTag("bob"), "stratus", map[string]cloud.Credential{
		"cred1": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		}),
		"cred2": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"b": "new b val",
		}),
		"cred4": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"d": "d val",
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	expect := map[string]cloud.Credential{
		"cred1": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		}),
		"cred2": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"b": "new b val",
		}),
		"cred3": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"user":     "bob",
			"password": "bob's password",
		}),
		"cred4": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"d": "d val",
		}),
	}
	addCredLabels(expect)

	creds1, err := s.State.CloudCredentials(names.NewUserTag("bob"), "stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds1, jc.DeepEquals, expect)
}

func (s *CloudCredentialsSuite) TestUpdateCloudCredentialsInvalidAuthType(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	err = s.State.UpdateCloudCredentials(names.NewUserTag("bob"), "stratus", map[string]cloud.Credential{
		"cred1": cloud.NewCredential(cloud.UserPassAuthType, nil),
	})
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials for user "user-bob", cloud "stratus": validating cloud credentials: credential "cred1" with auth-type "userpass" is not supported \(expected one of \["access-key"\]\)`)
}

// addCredLabels adds labels to all the given credentials, because
// the labels are present when the credentials are returned from the
// state but not when created with NewCredential.
func addCredLabels(creds map[string]cloud.Credential) {
	for name, cred := range creds {
		cred.Label = name
		creds[name] = cred
	}
}
