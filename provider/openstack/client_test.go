// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v3/client"
	"github.com/go-goose/goose/v3/identity"
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/testing"
)

type clientSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestFactoryInit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 1)

	err := factory.Init()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestFactoryNova(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 1)

	err := factory.Init()
	c.Assert(err, jc.ErrorIsNil)

	nova, err := factory.Nova()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nova, gc.NotNil)
}

func (s *clientSuite) TestFactoryNeutron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 2)

	err := factory.Init()
	c.Assert(err, jc.ErrorIsNil)

	nova, err := factory.Neutron()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nova, gc.NotNil)
}

func (s *clientSuite) setupMockFactory(ctrl *gomock.Controller, times int) *ClientFactory {
	mockClient := NewMockAuthenticatingClient(ctrl)
	mockClient.EXPECT().SetRequiredServiceTypes([]string{"compute"}).Times(times)

	mockConfig := NewMockSSLHostnameConfig(ctrl)
	mockConfig.EXPECT().SSLHostnameVerification().Return(true).Times(times)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		CredAttrUserName:          "admin",
		CredAttrPassword:          "password",
		CredAttrProjectDomainName: "default",
		CredAttrUserDomainName:    "",
		CredAttrDomainName:        "default",
		CredAttrVersion:           "3",
	})

	spec := environscloudspec.CloudSpec{
		Region:     "default",
		Endpoint:   "https://sharedhost.foo:443/identity/v3/",
		Credential: &cred,
	}

	factory := NewClientFactory(spec, mockConfig)
	factory.clientFunc = makeClientFunc(mockClient)

	return factory
}

func makeClientFunc(mockClient *MockAuthenticatingClient) ClientFunc {
	return func(identity.Credentials, identity.AuthMode, ...ClientOption) (client.AuthenticatingClient, error) {
		return mockClient, nil
	}
}
