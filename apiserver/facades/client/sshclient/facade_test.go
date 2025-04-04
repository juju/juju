// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/sshclient"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type facadeSuite struct {
	backend    *MockBackend
	authorizer *MockAuthorizer

	modelConfigService *MockModelConfigService
	execService        *MockExecService
	stubService        *MockStubService

	controllerUUID string
	modelUUID      model.UUID
}

var _ = gc.Suite(&facadeSuite{})

func (s *facadeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = NewMockBackend(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.execService = NewMockExecService(ctrl)
	s.stubService = NewMockStubService(ctrl)

	return ctrl
}

func (s *facadeSuite) SetUpTest(c *gc.C) {
	s.controllerUUID = names.NewControllerTag(s.controllerUUID).Id()
	s.modelUUID = modeltesting.GenModelUUID(c)
}

func (s *facadeSuite) TestNonClientNotAllowed(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
	c.Assert(facade, gc.IsNil)
}

// TestNonAuthUserDenied tests that a user without admin non
// superuser permission cannot access a facade function.
func (s *facadeSuite) TestNonAuthUserDenied(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(apiservererrors.ErrPerm),
	)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewMachineTag("0").String()}, {names.NewUnitTag("app/0").String()}},
	}
	results, err := facade.PublicAddress(context.Background(), args)
	// Check this was an error permission
	c.Assert(err, gc.ErrorMatches, apiservererrors.ErrPerm.Error())
	c.Assert(results, gc.DeepEquals, params.SSHAddressResults{})
}

// TestSuperUserAuth tests that a user with superuser privilege
// can access a facade function.
func (s *facadeSuite) TestSuperUserAuth(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	machine0 := NewMockSSHMachine(ctrl)
	machine0.EXPECT().PublicAddress().AnyTimes().Return(network.NewSpaceAddress("1.1.1.1"), nil)
	s.backend.EXPECT().GetMachineForEntity("machine-0").Return(machine0, nil)
	s.backend.EXPECT().GetMachineForEntity("unit-app-0").Return(machine0, nil)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewMachineTag("0").String()}, {names.NewUnitTag("app/0").String()}},
	}
	results, err := facade.PublicAddress(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{{
			Address: "1.1.1.1",
		}, {
			Address: "1.1.1.1",
		}},
	})
}

func (s *facadeSuite) TestPublicAddress(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	machine0 := NewMockSSHMachine(ctrl)
	machine0.EXPECT().PublicAddress().Return(network.NewSpaceAddress("1.1.1.1"), nil)
	s.backend.EXPECT().GetMachineForEntity("machine-0").Return(machine0, nil)
	machine1 := NewMockSSHMachine(ctrl)
	machine1.EXPECT().PublicAddress().Return(network.NewSpaceAddress("3.3.3.3"), nil)
	s.backend.EXPECT().GetMachineForEntity("unit-app-0").Return(machine1, nil)
	s.backend.EXPECT().GetMachineForEntity("unit-foo-0").Return(nil, fmt.Errorf("entity %w", errors.NotFound))

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewMachineTag("0").String()}, {names.NewUnitTag("app/0").String()}, {names.NewUnitTag("foo/0").String()}},
	}
	results, err := facade.PublicAddress(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
			{Address: "3.3.3.3"},
			{Error: apiservertesting.NotFoundError("entity")},
		},
	})
}

func (s *facadeSuite) TestPrivateAddress(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	machine0 := NewMockSSHMachine(ctrl)
	machine0.EXPECT().PrivateAddress().Return(network.NewSpaceAddress("2.2.2.2"), nil)
	s.backend.EXPECT().GetMachineForEntity("machine-0").Return(machine0, nil)
	machine1 := NewMockSSHMachine(ctrl)
	machine1.EXPECT().PrivateAddress().Return(network.NewSpaceAddress("4.4.4.4"), nil)
	s.backend.EXPECT().GetMachineForEntity("unit-app-0").Return(machine1, nil)
	s.backend.EXPECT().GetMachineForEntity("unit-foo-0").Return(nil, fmt.Errorf("entity %w", errors.NotFound))

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewUnitTag("foo/0").String()}, {names.NewMachineTag("0").String()}, {names.NewUnitTag("app/0").String()}},
	}
	results, err := facade.PrivateAddress(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Error: apiservertesting.NotFoundError("entity")},
			{Address: "2.2.2.2"},
			{Address: "4.4.4.4"},
		},
	})
}

func (s *facadeSuite) TestAllAddresses(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	machine0Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("9.9.9.9", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("2.2.2.2", network.WithScope(network.ScopeCloudLocal)),
	}
	machine0LegacyAddresses := network.SpaceAddresses{
		network.NewSpaceAddress("0.1.2.3", network.WithScope(network.ScopeCloudLocal)),
	}
	machine0 := NewMockSSHMachine(ctrl)
	machine0.EXPECT().AllDeviceSpaceAddresses(gomock.Any()).Return(machine0Addresses, nil)
	machine0.EXPECT().Addresses().Return(machine0LegacyAddresses)
	s.backend.EXPECT().GetMachineForEntity("machine-0").Return(machine0, nil)

	machine1Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("10.10.10.10", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("3.3.3.3", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("4.4.4.4", network.WithScope(network.ScopeCloudLocal)),
	}
	machine1LegacyAddresses := network.SpaceAddresses{
		network.NewSpaceAddress("0.3.2.1", network.WithScope(network.ScopeCloudLocal)),
	}
	machine1 := NewMockSSHMachine(ctrl)
	machine1.EXPECT().AllDeviceSpaceAddresses(gomock.Any()).Return(machine1Addresses, nil)
	machine1.EXPECT().Addresses().Return(machine1LegacyAddresses)
	s.backend.EXPECT().GetMachineForEntity("unit-app-0").Return(machine1, nil)

	s.backend.EXPECT().GetMachineForEntity("unit-foo-0").Return(nil, fmt.Errorf("entity %w", errors.NotFound))

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewUnitTag("foo/0").String()}, {names.NewMachineTag("0").String()}, {names.NewUnitTag("app/0").String()}},
	}
	results, err := facade.AllAddresses(context.Background(), args)
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
}

func (s *facadeSuite) TestPublicKeys(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	machine0 := NewMockSSHMachine(ctrl)
	machine0.EXPECT().MachineTag().Return(names.NewMachineTag("0"))
	s.backend.EXPECT().GetMachineForEntity("machine-0").Return(machine0, nil)
	machine1 := NewMockSSHMachine(ctrl)
	machine1.EXPECT().MachineTag().Return(names.NewMachineTag("1"))
	s.backend.EXPECT().GetMachineForEntity("unit-app-0").Return(machine1, nil)
	s.backend.EXPECT().GetMachineForEntity("unit-foo-0").Return(nil, fmt.Errorf("entity %w", errors.NotFound))

	s.backend.EXPECT().GetSSHHostKeys(names.NewMachineTag("0")).Return(state.SSHHostKeys{"rsa0", "dsa0"}, nil)
	s.backend.EXPECT().GetSSHHostKeys(names.NewMachineTag("1")).Return(state.SSHHostKeys{"rsa1", "dsa1"}, nil)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{{names.NewMachineTag("0").String()}, {names.NewUnitTag("foo/0").String()}, {names.NewUnitTag("app/0").String()}},
	}
	results, err := facade.PublicKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{
			{PublicKeys: []string{"rsa0", "dsa0"}},
			{Error: apiservertesting.NotFoundError("entity")},
			{PublicKeys: []string{"rsa1", "dsa1"}},
		},
	})
}

func (s *facadeSuite) TestProxyTrue(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(false, map[string]any{
		"name":      "donotuse",
		"type":      "donotuse",
		"uuid":      "00000000-0000-0000-0000-000000000000",
		"proxy-ssh": "true",
	}))

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.Proxy(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.UseProxy, jc.IsTrue)
}

func (s *facadeSuite) TestProxyFalse(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil),
	)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(false, map[string]any{
		"name":      "donotuse",
		"type":      "donotuse",
		"uuid":      "00000000-0000-0000-0000-000000000000",
		"proxy-ssh": "false",
	}))

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.Proxy(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.UseProxy, jc.IsFalse)
}

func (s *facadeSuite) TestModelCredentialForSSHFailedNotAuthorized(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(apiservererrors.ErrPerm),
	)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.ModelCredentialForSSH(context.Background())
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.IsNil)
}

func (s *facadeSuite) TestModelCredentialForSSHFailedBadCredential(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

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

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(nil),
		s.execService.EXPECT().GetCAASUnitExecSecretToken(gomock.Any()).Return("token", nil),
		s.stubService.EXPECT().CloudSpec(gomock.Any()).Return(cloudSpec, nil),
	)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.ModelCredentialForSSH(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiservererrors.RestoreError(result.Error), gc.ErrorMatches, `cloud spec "name" has empty credential not valid`)
	c.Assert(result.Result, gc.IsNil)
}

func (s *facadeSuite) TestModelCredentialForSSH(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(nil)

	s.assertModelCredentialForSSH(c)
}

func (s *facadeSuite) TestModelCredentialForSSHAdminAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(nil)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission)

	s.assertModelCredentialForSSH(c)
}

func (s *facadeSuite) TestModelCredentialForSSHSuperuserAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(nil)

	s.assertModelCredentialForSSH(c)
}

func (s *facadeSuite) assertModelCredentialForSSH(c *gc.C) {
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

	gomock.InOrder(
		s.authorizer.EXPECT().AuthClient().Return(true),
		s.execService.EXPECT().GetCAASUnitExecSecretToken(gomock.Any()).Return("token", nil),
		s.stubService.EXPECT().CloudSpec(gomock.Any()).Return(cloudSpec, nil),
	)

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.ModelCredentialForSSH(context.Background())
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

func (s *facadeSuite) TestGetVirtualHostnameForEntity(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade, err := sshclient.InternalFacade(
		names.NewControllerTag(s.controllerUUID),
		names.NewModelTag(s.modelUUID.String()),
		s.backend,
		s.modelConfigService,
		s.execService,
		s.stubService,
		nil,
		s.authorizer,
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
			expected: fmt.Sprintf("0.%s.%s", names.NewModelTag(s.modelUUID.String()).Id(), virtualhostname.Domain),
		},
		{
			name:     "test with unit tag",
			tag:      names.NewUnitTag("unit/0").String(),
			expected: fmt.Sprintf("0.unit.%s.%s", names.NewModelTag(s.modelUUID.String()).Id(), virtualhostname.Domain),
		},
		{
			name:      "test with unit tag and container",
			tag:       names.NewUnitTag("unit/0").String(),
			container: &container,
			expected:  fmt.Sprintf("container.0.unit.%s.%s", names.NewModelTag(s.modelUUID.String()).Id(), virtualhostname.Domain),
		},
		{
			name:          "test with error",
			tag:           "error-tag",
			expectedError: "\"error-tag\" is not a valid tag",
		},
	}
	for _, t := range tests {
		ctx := context.Background()
		s.authorizer.EXPECT().HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Return(authentication.ErrorEntityMissingPermission).Times(1)
		s.authorizer.EXPECT().HasPermission(ctx, permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).Return(nil).Times(1)
		c.Log(t.name)
		res, err := facade.VirtualHostname(ctx, params.VirtualHostnameTargetArg{
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
