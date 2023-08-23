// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/poolmanager"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type storageSetUp interface {
	setupVolumes(c *gc.C)
	setupFilesystems(c *gc.C)
}

type provisionerSuite struct {
	jujutesting.ApiServerSuite

	storageSetUp

	st             *state.State
	resources      *common.Resources
	authorizer     *apiservertesting.FakeAuthorizer
	api            *storageprovisioner.StorageProvisionerAPIv4
	storageBackend storageprovisioner.StorageBackend

	controllerConfigGetter *mocks.MockControllerConfigGetter
}

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "dummy",
		"password": "secret",
	})
	s.CredentialService = apiservertesting.FixedCredentialGetter(&cred)
	s.ApiServerSuite.SetUpTest(c)
}

func (s *iaasProvisionerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.storageSetUp = s

func (s *provisionerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigGetter = mocks.NewMockControllerConfigGetter(ctrl)
	s.controllerConfigGetter.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil).AnyTimes()

	return ctrl
}
