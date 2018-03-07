// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	jujucloud "github.com/juju/juju/cloud"
	statetesting "github.com/juju/juju/state/testing"
)

type cloudSuite struct{}

var _ = gc.Suite(&cloudSuite{})

func (*cloudSuite) TestCachingCredentialSchemaGetterResultsCached(c *gc.C) {
	ctrl, getSchema := setupMock(c)
	defer ctrl.Finish()

	// Call multiple times.
	// Expectation only handles a single call and so will fail on an attempt to
	// get data from state multiple times.
	for i := 0; i < 2; i++ {
		schemas, err := getSchema("dummy")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(len(schemas), jc.GreaterThan, 0)
	}
}

func (*cloudSuite) TestCredentialInfoFromStateCredentialOmitsHiddenAttributes(c *gc.C) {
	ctrl, getSchema := setupMock(c)
	defer ctrl.Finish()

	cred := statetesting.CloudCredentialWithName("dummy", jujucloud.UserPassAuthType,
		map[string]string{"username": "user", "password": "secret sauce"},
	)

	info, err := common.CredentialInfoFromStateCredential(cred, false, getSchema)
	c.Assert(err, jc.ErrorIsNil)

	_, ok := info.Content.Attributes["password"]
	c.Check(ok, jc.IsFalse)
}

func (*cloudSuite) TestCredentialInfoFromStateCredentialIncludesHiddenAttributes(c *gc.C) {
	ctrl, getSchema := setupMock(c)
	defer ctrl.Finish()

	cred := statetesting.CloudCredentialWithName("dummy", jujucloud.UserPassAuthType,
		map[string]string{"username": "user", "password": "secret sauce"},
	)

	info, err := common.CredentialInfoFromStateCredential(cred, true, getSchema)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.Content.Attributes["password"], gc.Equals, "secret sauce")
}

func setupMock(c *gc.C) (*gomock.Controller, common.CredentialSchemaGetter) {
	ctrl := gomock.NewController(c)

	const cloudName = "dummy"
	cloud := jujucloud.Cloud{
		Name:      cloudName,
		Type:      cloudName,
		AuthTypes: []jujucloud.AuthType{jujucloud.UserPassAuthType},
	}

	mockState := statetesting.NewMockCloudAccessor(ctrl)
	mockState.EXPECT().Cloud(cloudName).Return(cloud, nil).Times(1)

	return ctrl, common.CachingCredentialSchemaGetter(mockState)
}
