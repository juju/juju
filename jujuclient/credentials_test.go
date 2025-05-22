// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type CredentialsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store       jujuclient.CredentialStore
	cloudName   string
	credentials cloud.CloudCredential
}

func TestCredentialsSuite(t *stdtesting.T) {
	tc.Run(t, &CredentialsSuite{})
}

func (s *CredentialsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileCredentialStore()
	s.cloudName = "testcloud"
	s.credentials = cloud.CloudCredential{
		DefaultCredential: "peter",
		DefaultRegion:     "east",
		AuthCredentials: map[string]cloud.Credential{
			"peter": cloud.NewCredential(cloud.AccessKeyAuthType, nil),
			"paul":  cloud.NewCredential(cloud.AccessKeyAuthType, nil),
		},
	}
}

func (s *CredentialsSuite) TestCredentialForCloudNoFile(c *tc.C) {
	found, err := s.store.CredentialForCloud(s.cloudName)
	c.Assert(err, tc.ErrorMatches, "credentials for cloud testcloud not found")
	c.Assert(found, tc.IsNil)
}

func (s *CredentialsSuite) TestCredentialForCloudNoneExists(c *tc.C) {
	writeTestCredentialsFile(c)
	found, err := s.store.CredentialForCloud(s.cloudName)
	c.Assert(err, tc.ErrorMatches, "credentials for cloud testcloud not found")
	c.Assert(found, tc.IsNil)
}

func (s *CredentialsSuite) TestCredentialForCloud(c *tc.C) {
	name := firstTestCloudName(c)
	found, err := s.store.CredentialForCloud(name)
	c.Assert(err, tc.ErrorIsNil)
	expected := s.getCredentials(c)[name]
	c.Assert(found, tc.DeepEquals, &expected)
}

func (s *CredentialsSuite) TestUpdateCredentialAddFirst(c *tc.C) {
	err := s.store.UpdateCredential(s.cloudName, s.credentials)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) TestUpdateCredentialAddNew(c *tc.C) {
	s.assertCredentialsNotExists(c)
	err := s.store.UpdateCredential(s.cloudName, s.credentials)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) TestUpdateCredential(c *tc.C) {
	s.cloudName = firstTestCloudName(c)

	err := s.store.UpdateCredential(s.cloudName, s.credentials)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) TestUpdateCredentialRemovesDefaultIfNecessary(c *tc.C) {
	s.cloudName = firstTestCloudName(c)

	store := jujuclient.NewFileCredentialStore()
	err := store.UpdateCredential(s.cloudName, s.credentials)
	c.Assert(err, tc.ErrorIsNil)
	newCreds := s.credentials
	// "peter" is the default credential
	delete(newCreds.AuthCredentials, "peter")
	err = store.UpdateCredential(s.cloudName, newCreds)
	c.Assert(err, tc.ErrorIsNil)
	creds, err := store.AllCredentials()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds[s.cloudName].DefaultCredential, tc.Equals, "")
}

func (s *CredentialsSuite) TestUpdateCredentialRemovesCloudWhenNoCredentialLeft(c *tc.C) {
	s.cloudName = firstTestCloudName(c)

	store := jujuclient.NewFileCredentialStore()
	err := store.UpdateCredential(s.cloudName, s.credentials)
	c.Assert(err, tc.ErrorIsNil)

	// delete all
	err = store.UpdateCredential(s.cloudName, cloud.CloudCredential{})
	c.Assert(err, tc.ErrorIsNil)

	creds, err := store.AllCredentials()
	c.Assert(err, tc.ErrorIsNil)

	_, exists := creds[s.cloudName]
	c.Assert(exists, tc.IsFalse)
}

func (s *CredentialsSuite) assertCredentialsNotExists(c *tc.C) {
	all := writeTestCredentialsFile(c)
	_, exists := all[s.cloudName]
	c.Assert(exists, tc.IsFalse)
}

func (s *CredentialsSuite) assertUpdateSucceeded(c *tc.C) {
	c.Assert(s.getCredentials(c)[s.cloudName], tc.DeepEquals, s.credentials)
}

func (s *CredentialsSuite) getCredentials(c *tc.C) map[string]cloud.CloudCredential {
	credentials, err := s.store.AllCredentials()
	c.Assert(err, tc.ErrorIsNil)
	return credentials
}

func firstTestCloudName(c *tc.C) string {
	all := writeTestCredentialsFile(c)
	for key := range all {
		return key
	}
	return ""
}
