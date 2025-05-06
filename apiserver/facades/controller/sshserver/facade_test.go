// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/sshserver"
	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/rpc/params"
	state "github.com/juju/juju/state"
)

var _ = gc.Suite(&sshServerSuite{})

type sshServerSuite struct {
	ctxMock       *MockContext
	backendMock   *MockBackend
	resourcesMock *MockResources
}

func (s *sshServerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctxMock = NewMockContext(ctrl)
	s.backendMock = NewMockBackend(ctrl)
	s.resourcesMock = NewMockResources(ctrl)
	return ctrl
}

func (s *sshServerSuite) TestAuth(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	authorizer := NewMockAuthorizer(ctrl)

	s.ctxMock.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := sshserver.NewExternalFacade(s.ctxMock)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *sshServerSuite) TestControllerConfig(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().ControllerConfig().Return(
		controller.Config{"hi": "bye"},
		nil,
	)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	cfg, err := f.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.DeepEquals, params.ControllerConfigResult{Config: params.ControllerConfig{"hi": "bye"}})
}

func (s *sshServerSuite) TestWatchControllerConfig(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	watcher := workertest.NewFakeWatcher(1, 0)
	watcher.Ping() // Send some changes

	s.ctxMock.EXPECT().Resources().Return(s.resourcesMock)
	s.backendMock.EXPECT().WatchControllerConfig().Return(watcher, nil)
	s.resourcesMock.EXPECT().Register(watcher).Return("id")

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	result, err := f.WatchControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.NotifyWatcherId, gc.Equals, "id")

	// Now we close the channel expecting err
	watcher.Close()
	s.backendMock.EXPECT().WatchControllerConfig().Return(watcher, nil)

	_, err = f.WatchControllerConfig()
	c.Assert(err, gc.ErrorMatches, "An error")
}

func (s *sshServerSuite) TestSSHServerHostKey(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().SSHServerHostKey().Return("hostkey", nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	key, err := f.SSHServerHostKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.Equals, params.StringResult{Result: "hostkey"})
}

func (s *sshServerSuite) TestHostKeyForTarget(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().HostKeyForVirtualHostname(gomock.Any()).Return([]byte("hostkey"), nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	key, err := f.VirtualHostKey(params.SSHVirtualHostKeyRequestArg{Hostname: "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, params.SSHHostKeyResult{HostKey: []byte("hostkey")})
}

func (s *sshServerSuite) TestAuthorizedKeysForModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().AuthorizedKeysForModel("abcd").Return(
		[]string{"key1", "key2"}, nil)

	s.backendMock.EXPECT().AuthorizedKeysForModel("not-existing").Return(
		[]string{""}, nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	testCases := []struct {
		name            string
		expectKeys      []string
		modelUUID       string
		expectedSuccess bool
		expectedError   string
	}{
		{
			name:            "test for key added to a model",
			modelUUID:       "abcd",
			expectKeys:      []string{"key1", "key2"},
			expectedSuccess: true,
		},
		{
			name:       "test for not-existing model",
			modelUUID:  "not-existing",
			expectKeys: []string{""},
		},
	}

	for _, tc := range testCases {
		c.Logf("test: %s", tc.name)
		arg := params.ListAuthorizedKeysArgs{
			ModelUUID: tc.modelUUID,
		}
		results, err := f.ListAuthorizedKeysForModel(arg)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Error, gc.IsNil)
		c.Assert(results.AuthorizedKeys, gc.DeepEquals, tc.expectKeys)
	}
}

func (s *sshServerSuite) TestResolveK8sExecInfo(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().K8sNamespaceAndPodName("abcd", "unit").Return(
		"namespace", "pod-name", nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	arg := params.SSHK8sExecArg{
		ModelUUID: "abcd",
		UnitName:  "unit",
	}
	result, err := f.ResolveK8sExecInfo(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Namespace, gc.Equals, "namespace")
	c.Assert(result.PodName, gc.Equals, "pod-name")
}

func (s *sshServerSuite) TestCheckSSHAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	userTag := names.NewUserTag("alice")
	modelUUID := uuid.NewString()

	s.ctxMock.EXPECT().Resources()
	s.backendMock.EXPECT().ModelAccess(userTag, modelUUID).Return(
		permission.UserAccess{Access: permission.AdminAccess}, nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	arg := params.CheckSSHAccessArg{
		User:        userTag.Id(),
		Destination: destination.String(),
	}

	result := f.CheckSSHAccess(arg)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, true)
}

func (s *sshServerSuite) TestCheckSSHAccessViaControllerAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	userTag := names.NewUserTag("alice")
	modelUUID := uuid.NewString()

	// Check a user with no model access but controller superuser access.
	s.ctxMock.EXPECT().Resources()
	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	s.backendMock.EXPECT().ModelAccess(userTag, modelUUID).Return(
		permission.UserAccess{}, errors.NotFound)
	s.backendMock.EXPECT().ControllerAccess(userTag).Return(
		permission.UserAccess{Access: permission.AdminAccess}, nil)

	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	arg := params.CheckSSHAccessArg{
		User:        userTag.Id(),
		Destination: destination.String(),
	}

	result := f.CheckSSHAccess(arg)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, true)

	// Check a user with read model access but controller superuser access.
	s.backendMock.EXPECT().ModelAccess(userTag, modelUUID).Return(
		permission.UserAccess{Access: permission.ReadAccess}, nil)
	s.backendMock.EXPECT().ControllerAccess(userTag).Return(
		permission.UserAccess{Access: permission.AdminAccess}, nil)

	result = f.CheckSSHAccess(arg)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, true)
}

func (s *sshServerSuite) TestCheckSSHAccessNoAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	userTag := names.NewUserTag("alice")
	modelUUID := uuid.NewString()

	s.ctxMock.EXPECT().Resources()
	s.backendMock.EXPECT().ModelAccess(userTag, modelUUID).Return(
		permission.UserAccess{}, errors.NotFound)
	s.backendMock.EXPECT().ControllerAccess(userTag).Return(
		permission.UserAccess{}, nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	arg := params.CheckSSHAccessArg{
		User:        userTag.Id(),
		Destination: destination.String(),
	}

	result := f.CheckSSHAccess(arg)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, false)
}

func (s *sshServerSuite) TestValidateVirtualHostname(c *gc.C) {
	container := "charm"
	unitName := "nginx/0"
	machineID := "0"
	modelUUID := uuid.NewString()

	machineDestination, err := virtualhostname.NewInfoMachineTarget(modelUUID, machineID)
	c.Assert(err, jc.ErrorIsNil)

	unitDestination, err := virtualhostname.NewInfoUnitTarget(modelUUID, unitName)
	c.Assert(err, jc.ErrorIsNil)

	containerDestination, err := virtualhostname.NewInfoContainerTarget(modelUUID, unitName, container)
	c.Assert(err, jc.ErrorIsNil)

	testCases := []struct {
		desc        string
		destination string
		setupMocks  func()
		expectedErr string
	}{
		{
			desc:        "Valid machine hostname",
			destination: machineDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().MachineExists(modelUUID, machineID).Return(true, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
		},
		{
			desc:        "Invalid model type",
			destination: machineDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeCAAS, nil)
			},
			expectedErr: `failed to validate destination: attempting to connect to a machine in a "caas" model not valid`,
		},
		{
			desc:        "Machine doesn't exist",
			destination: machineDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().MachineExists(modelUUID, machineID).Return(false, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
			expectedErr: `failed to validate destination: machine with ID 0 not found`,
		},
		{
			desc:        "Failed to check if machine exists",
			destination: machineDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().MachineExists(modelUUID, machineID).Return(false, errors.New("some error"))
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
			expectedErr: `failed to validate destination: some error`,
		},
		{
			desc:        "Valid unit hostname",
			destination: unitDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(true, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
		},
		{
			desc:        "Unit doesn't exist",
			destination: unitDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(false, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
			expectedErr: `failed to validate destination: unit "nginx/0" not found`,
		},
		{
			desc:        "Failed to check if unit exists",
			destination: unitDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(false, errors.New("some error"))
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
			expectedErr: `failed to validate destination: some error`,
		},
		{
			desc:        "Valid container hostname",
			destination: containerDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(true, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeCAAS, nil)
			},
		},
		{
			desc:        "Invalid model type",
			destination: containerDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeIAAS, nil)
			},
			expectedErr: `failed to validate destination: attempting to connect to a container in a "iaas" model not valid`,
		},
		{
			desc:        "Container unit doesn't exist",
			destination: containerDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(false, nil)
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeCAAS, nil)
			},
			expectedErr: `failed to validate destination: unit "nginx/0" not found`,
		},
		{
			desc:        "Failed to check if container unit exists",
			destination: containerDestination.String(),
			setupMocks: func() {
				s.backendMock.EXPECT().UnitExists(modelUUID, unitName).Return(false, errors.New("some error"))
				s.backendMock.EXPECT().ModelType(modelUUID).Return(state.ModelTypeCAAS, nil)
			},
			expectedErr: `failed to validate destination: some error`,
		},
	}
	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		testFunc := func() {
			// Use an anonymous function to enable deferring the mock controller
			// setup so that each subtest can have its own controller.
			ctrl := s.setupMocks(c)
			defer ctrl.Finish()

			s.ctxMock.EXPECT().Resources()
			f := sshserver.NewFacade(s.ctxMock, s.backendMock)

			tC.setupMocks()
			arg := params.ValidateVirtualHostnameArg{Hostname: tC.destination}
			result := f.ValidateVirtualHostname(arg)
			if tC.expectedErr != "" {
				c.Assert(result.Error, gc.ErrorMatches, tC.expectedErr)
			} else {
				c.Assert(result.Error, gc.IsNil)
			}
		}
		testFunc()
	}
}
