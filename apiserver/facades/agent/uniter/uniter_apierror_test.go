// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&uniterAPIErrorSuite{})

func (s *uniterAPIErrorSuite) SetupTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err := domainServices.Credential().UpdateCloudCredential(context.Background(), testing.DefaultCredentialId, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *uniterAPIErrorSuite) TestGetStorageStateError(c *gc.C) {
	uniter.PatchGetStorageStateError(s, errors.New("kaboom"))

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })

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
		ControllerConfigService: domainServices.ControllerConfig(),
		MachineService:          domainServices.Machine(),
		ModelConfigService:      domainServices.Config(),
		ModelInfoService:        domainServices.ModelInfo(),
		NetworkService:          domainServices.Network(),
		PortService:             domainServices.Port(),
		SecretService:           domainServices.Secret(),
		UnitStateService:        domainServices.UnitState(),
		StubService:             domainServices.Stub(),
	}

	_, err := uniter.NewUniterAPIWithServices(context.Background(), facadeContext, services)
	c.Assert(err, gc.ErrorMatches, "kaboom")
}
