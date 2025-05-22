// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	stdtesting "testing"

	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testing"
)

type clientSuite struct {
	testing.BaseSuite
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestFactoryInit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 1)

	err := factory.Init()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientSuite) TestFactoryNova(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 1)

	err := factory.Init()
	c.Assert(err, tc.ErrorIsNil)

	nova, err := factory.Nova()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nova, tc.NotNil)
}

func (s *clientSuite) TestFactoryNeutron(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := s.setupMockFactory(ctrl, 2)

	err := factory.Init()
	c.Assert(err, tc.ErrorIsNil)

	nova, err := factory.Neutron()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nova, tc.NotNil)
}

func (s *clientSuite) TestFactoryAuthFallbackSuccess(c *tc.C) {
	err := s.testFactoryAuthFallback(c, nil)
	c.Assert(err, tc.IsNil)
}

func (s *clientSuite) TestFactoryAuthFallbackError(c *tc.C) {
	err := s.testFactoryAuthFallback(c, errors.New("bad auth"))
	c.Assert(err, tc.ErrorMatches, "bad auth")
}

func (s *clientSuite) testFactoryAuthFallback(c *tc.C, authErr error) error {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockClient := NewMockAuthenticatingClient(ctrl)
	mockClient.EXPECT().SetRequiredServiceTypes([]string{"compute"}).AnyTimes()
	mockClient.EXPECT().IdentityAuthOptions().Return(identity.AuthOptions{
		{
			Mode:     identity.AuthUserPassV3,
			Endpoint: "https://sharedhost.foo:443/identity/v3/",
		},
	}, nil)
	mockClient.EXPECT().Authenticate().Return(authErr)

	mockConfig := NewMockSSLHostnameConfig(ctrl)
	mockConfig.EXPECT().SSLHostnameVerification().Return(true).AnyTimes()

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		CredAttrUserName:          "admin",
		CredAttrPassword:          "password",
		CredAttrProjectDomainName: "default",
		CredAttrUserDomainName:    "",
		CredAttrDomainName:        "default",
		CredAttrVersion:           "2",
	})

	spec := environscloudspec.CloudSpec{
		Region:     "default",
		Endpoint:   "https://sharedhost.foo:443/identity/v3/",
		Credential: &cred,
	}

	factory := NewClientFactory(spec, mockConfig)
	factory.clientFunc = makeClientFunc(mockClient)

	return factory.Init()
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
