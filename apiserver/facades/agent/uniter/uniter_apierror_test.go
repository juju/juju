// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	secretservice "github.com/juju/juju/domain/secret/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/juju/testing"
)

type uniterAPIErrorSuite struct {
	testing.ApiServerSuite
}

var _ = gc.Suite(&uniterAPIErrorSuite{})

func (s *uniterAPIErrorSuite) SetupTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	serviceFactory := s.ControllerServiceFactory(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	serviceFactory.Credential().UpdateCloudCredential(context.Background(), testing.DefaultCredentialId, cred)
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

	serviceFactory := s.ControllerServiceFactory(c)
	_, err := uniter.NewUniterAPIWithServices(
		context.Background(), facadeContext,
		serviceFactory.ControllerConfig(),
		serviceFactory.Config(),
		serviceFactory.ModelInfo(),
		serviceFactory.Secret(secretservice.NotImplementedBackendConfigGetter),
		serviceFactory.Network(),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(nil),
	)
	c.Assert(err, gc.ErrorMatches, "kaboom")
}
