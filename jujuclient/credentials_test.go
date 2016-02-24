// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type CredentialsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store       jujuclient.CredentialsStore
	cloudName   string
	credentials cloud.CloudCredential
}

var _ = gc.Suite(&CredentialsSuite{})

func (s *CredentialsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
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

func (s *CredentialsSuite) TestCredentialsForCloudNoFile(c *gc.C) {
	found, err := s.store.CredentialsForCloud(s.cloudName)
	c.Assert(err, gc.ErrorMatches, "credentials for cloud testcloud not found")
	c.Assert(found, gc.IsNil)
}

func (s *CredentialsSuite) TestCredentialsForCloudNoneExists(c *gc.C) {
	writeTestCredentialsFile(c)
	found, err := s.store.CredentialsForCloud(s.cloudName)
	c.Assert(err, gc.ErrorMatches, "credentials for cloud testcloud not found")
	c.Assert(found, gc.IsNil)
}

func (s *CredentialsSuite) TestCredentialsForCloud(c *gc.C) {
	name := firstTestCloudName(c)
	found, err := s.store.CredentialsForCloud(name)
	c.Assert(err, jc.ErrorIsNil)
	expected := s.getCredentials(c)[name]
	c.Assert(found, gc.DeepEquals, expected)
}

func (s *CredentialsSuite) TestUpdateCredentialsAddFirst(c *gc.C) {
	err := s.store.UpdateCredentials(s.cloudName, s.credentials)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) TestUpdateCredentialsAddNew(c *gc.C) {
	s.assertCredentialsNotExists(c)
	err := s.store.UpdateCredentials(s.cloudName, s.credentials)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) TestUpdateCredentials(c *gc.C) {
	s.cloudName = firstTestCloudName(c)

	err := s.store.UpdateCredentials(s.cloudName, s.credentials)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *CredentialsSuite) assertCredentialsNotExists(c *gc.C) {
	all := writeTestCredentialsFile(c)
	_, exists := all[s.cloudName]
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialsSuite) assertUpdateSucceeded(c *gc.C) {
	c.Assert(s.getCredentials(c)[s.cloudName], gc.DeepEquals, &s.credentials)
}

func (s *CredentialsSuite) getCredentials(c *gc.C) map[string]*cloud.CloudCredential {
	credentials, err := s.store.AllCredentials()
	c.Assert(err, jc.ErrorIsNil)
	return credentials
}

func firstTestCloudName(c *gc.C) string {
	all := writeTestCredentialsFile(c)
	for key, _ := range all {
		return key
	}
	return ""
}
