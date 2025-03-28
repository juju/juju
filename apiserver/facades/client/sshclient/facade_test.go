// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/sshclient"
	"github.com/juju/juju/apiserver/facades/client/sshclient/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type facadeSuite struct {
	testing.BaseSuite
	backend          *mockBackend
	authorizer       *apiservertesting.FakeAuthorizer
	facade           *sshclient.Facade
	m0, uFoo, uOther string

	callContext environscontext.ProviderCallContext
}

var _ = gc.Suite(&facadeSuite{})

func (s *facadeSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.m0 = names.NewMachineTag("0").String()
	s.uFoo = names.NewUnitTag("foo/0").String()
	s.uOther = names.NewUnitTag("other/1").String()
}

func (s *facadeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.backend = new(mockBackend)
	s.authorizer = new(apiservertesting.FakeAuthorizer)
	s.authorizer.Tag = names.NewUserTag("igor")
	s.authorizer.AdminTag = names.NewUserTag("igor")

	facade, err := sshclient.InternalFacade(s.backend, nil, s.authorizer, s.callContext, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *facadeSuite) TestMachineAuthNotAllowed(c *gc.C) {
	s.authorizer.Tag = names.NewMachineTag("0")
	_, err := sshclient.InternalFacade(s.backend, nil, s.authorizer, s.callContext, nil)
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
}

func (s *facadeSuite) TestUnitAuthNotAllowed(c *gc.C) {
	s.authorizer.Tag = names.NewUnitTag("foo/0")
	_, err := sshclient.InternalFacade(s.backend, nil, s.authorizer, s.callContext, nil)
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
}

// TestNonAuthUserDenied tests that a user without admin non
// superuser permission cannot access a facade function.
func (s *facadeSuite) TestNonAuthUserDenied(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("jeremy")
	s.authorizer.AdminTag = names.NewUserTag("igor")

	facade, err := sshclient.InternalFacade(s.backend, nil, s.authorizer, s.callContext, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade

	args := params.Entities{
		Entities: []params.Entity{{s.m0}, {s.uFoo}, {s.uOther}},
	}
	results, err := s.facade.PublicAddress(args)
	// Check this was an error permission
	c.Assert(err, gc.ErrorMatches, apiservererrors.ErrPerm.Error())
	c.Assert(results, gc.DeepEquals, params.SSHAddressResults{})
}

// TestSuperUserAuth tests that a user with superuser privilege
// can access a facade function.
func (s *facadeSuite) TestSuperUserAuth(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("superuser-jeremy")
	s.authorizer.AdminTag = names.NewUserTag("igor")

	facade, err := sshclient.InternalFacade(s.backend, nil, s.authorizer, s.callContext, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade

	args := params.Entities{
		Entities: []params.Entity{{s.m0}, {s.uFoo}, {s.uOther}},
	}
	results, err := s.facade.PublicAddress(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
			{Address: "3.3.3.3"},
			{Error: apiservertesting.NotFoundError("entity")},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"GetMachineForEntity", []interface{}{s.m0}},
		{"GetMachineForEntity", []interface{}{s.uFoo}},
		{"GetMachineForEntity", []interface{}{s.uOther}},
	})

}

func (s *facadeSuite) TestPublicAddress(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{s.m0}, {s.uFoo}, {s.uOther}},
	}
	results, err := s.facade.PublicAddress(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
			{Address: "3.3.3.3"},
			{Error: apiservertesting.NotFoundError("entity")},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"GetMachineForEntity", []interface{}{s.m0}},
		{"GetMachineForEntity", []interface{}{s.uFoo}},
		{"GetMachineForEntity", []interface{}{s.uOther}},
	})
}

func (s *facadeSuite) TestPrivateAddress(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{s.uOther}, {s.m0}, {s.uFoo}},
	}
	results, err := s.facade.PrivateAddress(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Error: apiservertesting.NotFoundError("entity")},
			{Address: "2.2.2.2"},
			{Address: "4.4.4.4"},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"GetMachineForEntity", []interface{}{s.uOther}},
		{"GetMachineForEntity", []interface{}{s.m0}},
		{"GetMachineForEntity", []interface{}{s.uFoo}},
	})
}

func (s *facadeSuite) TestAllAddresses(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{s.uOther}, {s.m0}, {s.uFoo}},
	}
	results, err := s.facade.AllAddresses(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressesResults{
		Results: []params.SSHAddressesResult{
			{Error: apiservertesting.NotFoundError("entity")},
			// Addresses include those from both the machine and devices.
			// Sorted by scope - public first, then cloud local.
			// Then sorted lexically within the same scope.
			{Addresses: []string{
				"1.1.1.1",
				"9.9.9.9",
				"0.1.2.3",
				"2.2.2.2",
			}},
			{Addresses: []string{
				"10.10.10.10",
				"3.3.3.3",
				"0.3.2.1",
				"4.4.4.4",
			}},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"GetMachineForEntity", []interface{}{s.uOther}},
		{"GetMachineForEntity", []interface{}{s.m0}},
		{"GetMachineForEntity", []interface{}{s.uFoo}},
	})
}

func (s *facadeSuite) TestPublicKeys(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{s.m0}, {s.uOther}, {s.uFoo}},
	}
	results, err := s.facade.PublicKeys(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{
			{PublicKeys: []string{"rsa0", "dsa0"}},
			{Error: apiservertesting.NotFoundError("entity")},
			{PublicKeys: []string{"rsa1", "dsa1"}},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"GetMachineForEntity", []interface{}{s.m0}},
		{"GetSSHHostKeys", []interface{}{names.NewMachineTag("0")}},
		{"GetMachineForEntity", []interface{}{s.uOther}},
		{"GetMachineForEntity", []interface{}{s.uFoo}},
		{"GetSSHHostKeys", []interface{}{names.NewMachineTag("1")}},
	})
}

func (s *facadeSuite) TestProxyTrue(c *gc.C) {
	s.backend.proxySSH = true
	result, err := s.facade.Proxy()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.UseProxy, jc.IsTrue)
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"ModelConfig", []interface{}{}},
	})
}

func (s *facadeSuite) TestProxyFalse(c *gc.C) {
	s.backend.proxySSH = false
	result, err := s.facade.Proxy()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.UseProxy, jc.IsFalse)
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{
		{"ModelConfig", []interface{}{}},
	})
}

func (s *facadeSuite) TestModelCredentialForSSHFailedNotAuthorized(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	backend := mocks.NewMockBackend(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	broker := mocks.NewMockBroker(ctrl)

	backend.EXPECT().ModelTag().Return(testing.ModelTag)
	backend.EXPECT().ControllerTag().Return(testing.ControllerTag)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		authorizer.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(apiservererrors.ErrPerm),
	)
	facade, err := sshclient.InternalFacade(backend, nil, authorizer, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return broker, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := facade.ModelCredentialForSSH()
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.IsNil)
}

func (s *facadeSuite) TestModelCredentialForSSHFailedNonCAASModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	broker := mocks.NewMockBroker(ctrl)

	backend.EXPECT().ModelTag().Return(testing.ModelTag)
	backend.EXPECT().ControllerTag().Return(testing.ControllerTag)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		authorizer.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil),
		backend.EXPECT().Model().Return(model, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
	)
	facade, err := sshclient.InternalFacade(backend, nil, authorizer, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return broker, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := facade.ModelCredentialForSSH()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiservererrors.RestoreError(result.Error), gc.ErrorMatches, `facade ModelCredentialForSSH for non "caas" model not supported`)
	c.Assert(result.Result, gc.IsNil)
}

func (s *facadeSuite) TestModelCredentialForSSHFailedBadCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	broker := mocks.NewMockBroker(ctrl)

	cloudSpec := environscloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		CACertificates:   []string{testing.CACert},
		SkipTLSVerify:    true,
	}

	backend.EXPECT().ModelTag().Return(testing.ModelTag)
	backend.EXPECT().ControllerTag().Return(testing.ControllerTag)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		authorizer.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil),
		backend.EXPECT().Model().Return(model, nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
		backend.EXPECT().CloudSpec().Return(cloudSpec, nil),
	)
	facade, err := sshclient.InternalFacade(backend, nil, authorizer, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return broker, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := facade.ModelCredentialForSSH()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiservererrors.RestoreError(result.Error), gc.ErrorMatches, `cloud spec "name" has empty credential not valid`)
	c.Assert(result.Result, gc.IsNil)
}

func (s *facadeSuite) TestModelCredentialForSSH(c *gc.C) {
	s.assertModelCredentialForSSH(c,
		func(authorizer *mocks.MockAuthorizer) {
			authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
			authorizer.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil)
		},
	)
}

func (s *facadeSuite) TestModelCredentialForSSHAdminAccess(c *gc.C) {
	s.assertModelCredentialForSSH(c,
		func(authorizer *mocks.MockAuthorizer) {
			authorizer.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil)
			authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
		},
	)
}

func (s *facadeSuite) TestModelCredentialForSSHSuperuserAccess(c *gc.C) {
	s.assertModelCredentialForSSH(c,
		func(authorizer *mocks.MockAuthorizer) {
			authorizer.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(nil)
		},
	)
}

func (s *facadeSuite) assertModelCredentialForSSH(c *gc.C, f func(authorizer *mocks.MockAuthorizer)) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	f(authorizer)
	broker := mocks.NewMockBroker(ctrl)

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{
			k8scloud.CredAttrUsername: "foo",
			k8scloud.CredAttrPassword: "pwd",
		},
	)
	cloudSpec := environscloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential:       &credential,
		CACertificates:   []string{testing.CACert},
		SkipTLSVerify:    true,
	}

	backend.EXPECT().ModelTag().Return(testing.ModelTag).AnyTimes()
	backend.EXPECT().ControllerTag().Return(testing.ControllerTag)
	model.EXPECT().ControllerUUID().Return(testing.ControllerTag.Id())

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().Model().Return(model, nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
		backend.EXPECT().CloudSpec().Return(cloudSpec, nil),
		model.EXPECT().Config().Return(nil, nil),
		broker.EXPECT().GetSecretToken(k8sprovider.ExecRBACResourceName).Return("token", nil),
	)
	facade, err := sshclient.InternalFacade(backend, nil, authorizer, s.callContext,
		func(_ context.Context, arg environs.OpenParams) (sshclient.Broker, error) {
			c.Assert(arg.ControllerUUID, gc.Equals, testing.ControllerTag.Id())
			c.Assert(arg.Cloud, gc.DeepEquals, cloudSpec)
			return broker, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := facade.ModelCredentialForSSH()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, &params.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential: &params.CloudCredential{
			AuthType: "auth-type",
			Attributes: map[string]string{
				k8scloud.CredAttrUsername: "",
				k8scloud.CredAttrPassword: "",
				k8scloud.CredAttrToken:    "token",
			},
		},
		CACertificates: []string{testing.CACert},
		SkipTLSVerify:  true,
	})
}

type mockBackend struct {
	stub     jujutesting.Stub
	proxySSH bool
}

func (backend *mockBackend) JumpServerVirtualPublicKey() (string, error) {
	return "", errors.NotImplementedf("JumpServerVirtualPublicKey")
}

func (backend *mockBackend) MachineVirtualPublicKey(string) (string, error) {
	return "", errors.NotImplementedf("MachineVirtualPublicKey")
}

func (backend *mockBackend) UnitVirtualPublicKey(string) (string, error) {
	return "", errors.NotImplementedf("UnitVirtualPublicKey")
}

func (backend *mockBackend) ModelTag() names.ModelTag {
	return testing.ModelTag
}

func (backend *mockBackend) CloudSpec() (environscloudspec.CloudSpec, error) {
	return environscloudspec.CloudSpec{}, errors.NotImplementedf("CloudSpec")
}

func (backend *mockBackend) Model() (sshclient.Model, error) {
	return nil, errors.NotImplementedf("CloudSpec")
}

func (backend *mockBackend) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
}

func (backend *mockBackend) ModelConfig() (*config.Config, error) {
	backend.stub.AddCall("ModelConfig")
	attrs := testing.FakeConfig()
	attrs["proxy-ssh"] = backend.proxySSH
	conf, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conf, nil
}

func (backend *mockBackend) GetMachineForEntity(tagString string) (sshclient.SSHMachine, error) {
	backend.stub.AddCall("GetMachineForEntity", tagString)
	switch tagString {
	case names.NewMachineTag("0").String():
		return &mockMachine{
			tag:            names.NewMachineTag("0"),
			publicAddress:  "1.1.1.1",
			privateAddress: "2.2.2.2",
			addresses: network.SpaceAddresses{
				network.NewSpaceAddress("9.9.9.9", network.WithScope(network.ScopePublic)),
			},
			allNetworkAddresses: network.SpaceAddresses{
				network.NewSpaceAddress("0.1.2.3", network.WithScope(network.ScopeCloudLocal)),
				network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopePublic)),
				network.NewSpaceAddress("2.2.2.2", network.WithScope(network.ScopeCloudLocal)),
			},
		}, nil
	case names.NewUnitTag("foo/0").String():
		return &mockMachine{
			tag:            names.NewMachineTag("1"),
			publicAddress:  "3.3.3.3",
			privateAddress: "4.4.4.4",
			addresses: network.SpaceAddresses{
				network.NewSpaceAddress("10.10.10.10", network.WithScope(network.ScopePublic)),
			},
			allNetworkAddresses: network.SpaceAddresses{
				network.NewSpaceAddress("0.3.2.1", network.WithScope(network.ScopeCloudLocal)),
				network.NewSpaceAddress("3.3.3.3", network.WithScope(network.ScopePublic)),
				network.NewSpaceAddress("4.4.4.4", network.WithScope(network.ScopeCloudLocal)),
			},
		}, nil
	}
	return nil, errors.NotFoundf("entity")
}

func (backend *mockBackend) GetSSHHostKeys(tag names.MachineTag) (state.SSHHostKeys, error) {
	backend.stub.AddCall("GetSSHHostKeys", tag)
	switch tag {
	case names.NewMachineTag("0"):
		return state.SSHHostKeys{"rsa0", "dsa0"}, nil
	case names.NewMachineTag("1"):
		return state.SSHHostKeys{"rsa1", "dsa1"}, nil
	}
	return nil, errors.New("machine not found")
}

type mockMachine struct {
	tag            names.MachineTag
	publicAddress  string
	privateAddress string

	addresses           network.SpaceAddresses
	allNetworkAddresses network.SpaceAddresses
}

func (m *mockMachine) MachineTag() names.MachineTag {
	return m.tag
}

func (m *mockMachine) PublicAddress() (network.SpaceAddress, error) {
	return network.NewSpaceAddress(m.publicAddress, network.WithScope(network.ScopePublic)), nil
}

func (m *mockMachine) PrivateAddress() (network.SpaceAddress, error) {
	return network.NewSpaceAddress(m.privateAddress, network.WithScope(network.ScopeCloudLocal)), nil
}

func (m *mockMachine) AllDeviceSpaceAddresses() (network.SpaceAddresses, error) {
	return m.allNetworkAddresses, nil
}

func (m *mockMachine) Addresses() network.SpaceAddresses {
	return m.addresses
}

type facadeSuiteNewMocks struct {
	testing.BaseSuite

	callContext environscontext.ProviderCallContext

	mockBackend    *mocks.MockBackend
	mockAuthoriser *mocks.MockAuthorizer
	mockBroker     *mocks.MockBroker
}

var _ = gc.Suite(&facadeSuiteNewMocks{})

func (s *facadeSuiteNewMocks) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockBackend = mocks.NewMockBackend(ctrl)
	s.mockAuthoriser = mocks.NewMockAuthorizer(ctrl)
	s.mockBroker = mocks.NewMockBroker(ctrl)

	return ctrl
}

func (s *facadeSuiteNewMocks) TestGetVirtualHostnameForEntity(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.mockBackend.EXPECT().ModelTag().Return(testing.ModelTag).AnyTimes()
	s.mockBackend.EXPECT().ControllerTag().Return(testing.ControllerTag).AnyTimes()
	s.mockAuthoriser.EXPECT().AuthClient().Return(true).Times(1)

	facade, err := sshclient.InternalFacade(s.mockBackend, nil, s.mockAuthoriser, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return s.mockBroker, nil
		},
	)

	c.Assert(err, jc.ErrorIsNil)
	container := "container"
	tests := []struct {
		name          string
		tag           string
		container     *string
		expected      string
		expectedError string
	}{
		{
			name:     "test with machine tag",
			tag:      names.NewMachineTag("0").String(),
			expected: fmt.Sprintf("0.%s.%s", testing.ModelTag.Id(), virtualhostname.Domain),
		},
		{
			name:     "test with unit tag",
			tag:      names.NewUnitTag("unit/0").String(),
			expected: fmt.Sprintf("0.unit.%s.%s", testing.ModelTag.Id(), virtualhostname.Domain),
		},
		{
			name:      "test with unit tag and container",
			tag:       names.NewUnitTag("unit/0").String(),
			container: &container,
			expected:  fmt.Sprintf("container.0.unit.%s.%s", testing.ModelTag.Id(), virtualhostname.Domain),
		},
		{
			name:          "test with error",
			tag:           "error-tag",
			expectedError: "\"error-tag\" is not a valid tag",
		},
	}
	for _, t := range tests {
		s.mockAuthoriser.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission).Times(1)
		s.mockAuthoriser.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil).Times(1)
		c.Log(t.name)
		res, err := facade.VirtualHostname(params.VirtualHostnameTargetArg{
			Tag:       t.tag,
			Container: t.container,
		})
		if t.expectedError != "" {
			c.Assert(err, gc.ErrorMatches, t.expectedError)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(res.Address, gc.Equals, t.expected)
		}
	}
}

func (s *facadeSuiteNewMocks) TestPublicHostKeyForTarget(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.mockAuthoriser.EXPECT().AuthClient().Return(true).Times(1)

	facade, err := sshclient.InternalFacade(s.mockBackend, nil, s.mockAuthoriser, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return s.mockBroker, nil
		},
	)
	c.Assert(err, gc.IsNil)

	// Set superuser for all calls.
	controllerTag := names.NewControllerTag("controller00-0000-0000-000000000000")
	s.mockBackend.EXPECT().ControllerTag().Return(controllerTag).AnyTimes()
	s.mockAuthoriser.EXPECT().HasPermission(permission.SuperuserAccess, controllerTag).AnyTimes()

	// Test container target hits correct methods.
	gomock.InOrder(
		s.mockBackend.EXPECT().UnitVirtualPublicKey("postgresql/1").Return("postgres-container-host-key", nil).Times(1),
		s.mockBackend.EXPECT().JumpServerVirtualPublicKey().Return("server-host-key", nil).Times(1),
	)
	res := facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res, gc.Equals, params.PublicSSHHostKeyResult{
		Error:               nil,
		PublicKey:           "postgres-container-host-key",
		JumpServerPublicKey: "server-host-key",
	})

	// Test unit target hits correct methods.
	gomock.InOrder(
		s.mockBackend.EXPECT().UnitVirtualPublicKey("openfga/1").Return("openfga-container-host-key", nil).Times(1),
		s.mockBackend.EXPECT().JumpServerVirtualPublicKey().Return("server-host-key", nil).Times(1),
	)
	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.openfga.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res, gc.Equals, params.PublicSSHHostKeyResult{
		Error:               nil,
		PublicKey:           "openfga-container-host-key",
		JumpServerPublicKey: "server-host-key",
	})

	// Test machine target hits correct methods.
	gomock.InOrder(
		s.mockBackend.EXPECT().MachineVirtualPublicKey("1").Return("machine-host-key", nil).Times(1),
		s.mockBackend.EXPECT().JumpServerVirtualPublicKey().Return("server-host-key", nil).Times(1),
	)
	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res, gc.Equals, params.PublicSSHHostKeyResult{
		Error:               nil,
		PublicKey:           "machine-host-key",
		JumpServerPublicKey: "server-host-key",
	})
}

func (s *facadeSuiteNewMocks) TestPublicHostKeyForTargetErrors(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.mockAuthoriser.EXPECT().AuthClient().Return(true).Times(1)

	facade, err := sshclient.InternalFacade(s.mockBackend, nil, s.mockAuthoriser, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return s.mockBroker, nil
		},
	)
	c.Assert(err, gc.IsNil)

	// Set superuser for all calls.
	controllerTag := names.NewControllerTag("controller00-0000-0000-000000000000")
	s.mockBackend.EXPECT().ControllerTag().Return(controllerTag).AnyTimes()
	s.mockAuthoriser.EXPECT().HasPermission(permission.SuperuserAccess, controllerTag).AnyTimes()

	// Test failure to parse hostname.
	res := facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "BLAAAH 1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res.Error, gc.ErrorMatches, "failed to parse hostname: could not parse hostname")

	// Test MachineVirtualHostKey fail.
	gomock.InOrder(
		s.mockBackend.EXPECT().MachineVirtualPublicKey("1").Return("", errors.New("an-error")).Times(1),
	)
	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res.Error, gc.ErrorMatches, "failed to get machine host key: an-error")

	// Test UnitVirtualHostKey fail.
	gomock.InOrder(
		s.mockBackend.EXPECT().UnitVirtualPublicKey("openfga/1").Return("", errors.New("an-error")).Times(1),
	)
	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.openfga.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res.Error, gc.ErrorMatches, "failed to get unit host key: an-error")

	// Test SSHServerHostKey fail.
	gomock.InOrder(
		s.mockBackend.EXPECT().UnitVirtualPublicKey("postgresql/1").Return("postgres-container-host-key", nil).Times(1),
		s.mockBackend.EXPECT().JumpServerVirtualPublicKey().Return("", errors.New("an-error")).Times(1),
	)
	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res.Error, gc.ErrorMatches, "failed to get controller jumpserver host key: an-error")
}

func (s *facadeSuiteNewMocks) TestPublicHostKeyForTargetUserAuth(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.mockAuthoriser.EXPECT().AuthClient().Return(true).Times(1)

	facade, err := sshclient.InternalFacade(s.mockBackend, nil, s.mockAuthoriser, s.callContext,
		func(context.Context, environs.OpenParams) (sshclient.Broker, error) {
			return s.mockBroker, nil
		},
	)
	c.Assert(err, gc.IsNil)

	controllerTag := names.NewControllerTag("controller00-0000-0000-000000000000")
	s.mockBackend.EXPECT().ControllerTag().Return(controllerTag).AnyTimes()

	// Test a none ErrorEntityMissingPermission error on superuser check and is unrelated
	// to permission errors.
	s.mockAuthoriser.EXPECT().HasPermission(permission.SuperuserAccess, controllerTag).Return(errors.New("an-error")).Times(1)
	res := facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.openfga.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(res.Error, gc.ErrorMatches, "an-error")

	// Test second has permission for model reader is called and also has no permission/returns error.
	modelTag := names.NewModelTag("model000-0000-0000-0000-000000000000")

	s.mockAuthoriser.EXPECT().HasPermission(permission.SuperuserAccess, controllerTag).Return(authentication.ErrorEntityMissingPermission).Times(1)
	s.mockBackend.EXPECT().ModelTag().Return(modelTag)
	s.mockAuthoriser.EXPECT().HasPermission(permission.ReadAccess, modelTag).Return(authentication.ErrorEntityMissingPermission).Times(1)

	res = facade.PublicHostKeyForTarget(params.SSHHostKeyRequestArg{
		Hostname: "1.openfga.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	})
	c.Assert(*res.Error, gc.ErrorMatches, "entity missing permission")
}
