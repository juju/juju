// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/juju/testing"
)

type uniterAPIErrorSuite struct {
	testing.ApiServerSuite
}

func TestUniterAPIErrorSuite(t *stdtesting.T) {
	tc.Run(t, &uniterAPIErrorSuite{})
}

func (s *uniterAPIErrorSuite) SetupTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err := domainServices.Credential().UpdateCloudCredential(c.Context(), testing.DefaultCredentialId, cred)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *uniterAPIErrorSuite) TestGetStorageStateError(c *tc.C) {
	uniter.PatchGetStorageStateError(s, errors.New("kaboom"))

	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })

	facadeContext := facadetest.ModelContext{
		State_:             s.ControllerModel(c).State(),
		StatePool_:         s.StatePool(),
		Resources_:         resources,
		Auth_:              apiservertesting.FakeAuthorizer{Tag: names.NewUnitTag("nomatter/0")},
		LeadershipChecker_: &fakeLeadershipChecker{isLeader: false},
		Logger_:            loggertesting.WrapCheckLog(c),
	}

	domainServices := s.ControllerDomainServices(c)
	services := uniter.Services{
		ApplicationService:      domainServices.Application(),
		ResolveService:          domainServices.Resolve(),
		ControllerConfigService: domainServices.ControllerConfig(),
		MachineService:          domainServices.Machine(),
		ModelConfigService:      domainServices.Config(),
		ModelInfoService:        domainServices.ModelInfo(),
		PortService:             domainServices.Port(),
		SecretService:           domainServices.Secret(),
		UnitStateService:        domainServices.UnitState(),
		ModelProviderService:    domainServices.ModelProvider(),
	}

	_, err := uniter.NewUniterAPIWithServices(c.Context(), facadeContext, services)
	c.Assert(err, tc.ErrorMatches, "kaboom")
}
