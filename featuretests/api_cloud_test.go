// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/testing"
)

// NOTE(axw) this suite only exists because nothing exercises
// the cloud API enough to expose serialisation bugs such as
// lp:1607557. If/when we have commands that would expose that
// bug, we should drop this suite and write a new command-based
// one.

type CloudAPISuite struct {
	testing.JujuConnSuite
	client *apicloud.Client
}

func (s *CloudAPISuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = apicloud.NewClient(s.OpenControllerAPI(c))
}

func (s *CloudAPISuite) TearDownTest(c *gc.C) {
	s.client.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *CloudAPISuite) TestCloudAPI(c *gc.C) {
	result, err := s.client.Cloud(names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions: []cloud.Region{
			{
				Name:             "dummy-region",
				Endpoint:         "dummy-endpoint",
				IdentityEndpoint: "dummy-identity-endpoint",
				StorageEndpoint:  "dummy-storage-endpoint",
			},
		},
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		StorageEndpoint:  "dummy-storage-endpoint",
	})
}

func (s *CloudAPISuite) TestCredentialsAPI(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/admin/default")
	_, err := s.client.UpdateCredentialsCheckModels(tag, cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "fred", "password": "secret"},
	))
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType:   "userpass",
			Attributes: map[string]string{"username": "fred"},
			Redacted:   []string{"password"},
		}},
	})
}
