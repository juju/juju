// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	apicrossmodel "github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

const (
	serviceCall       = "serviceCall"
	environConfigCall = "environConfigCall"
	environUUIDCall   = "environUUIDCall"
)

type baseCrossmodelSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *apicrossmodel.API

	serviceDirectory *mockServiceDirectory
	stateAccess      *mockStateAccess
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.serviceDirectory = &mockServiceDirectory{}
	s.stateAccess = &mockStateAccess{}

	var err error
	s.api, err = apicrossmodel.CreateAPI(s.serviceDirectory, s.stateAccess, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

type mockStateAccess struct {
	jtesting.Stub

	services map[string]*state.Service
}

func (s *mockStateAccess) addService(c *gc.C, factory jujutesting.JujuConnSuite, name string) crossmodel.ServiceOffer {
	ch := factory.AddTestingCharm(c, "wordpress")

	if s.services == nil {
		s.services = make(map[string]*state.Service)
	}
	s.services[name] = factory.AddTestingService(c, name, ch)

	cfg, _ := mockEnvironConfig()
	uuid, _ := cfg.UUID()
	return crossmodel.ServiceOffer{
		ServiceName:        name,
		ServiceDescription: ch.Meta().Description,
		SourceLabel:        cfg.Name(),
		SourceEnvUUID:      uuid,
		Endpoints:          []charm.Relation{},
	}
}

func (s *mockStateAccess) Service(name string) (*state.Service, error) {
	s.AddCall(serviceCall)

	service, ok := s.services[name]
	if !ok {
		return nil, errors.Errorf("cannot get service %q", name)
	}
	return service, nil
}

func (s *mockStateAccess) EnvironConfig() (*config.Config, error) {
	s.AddCall(environConfigCall)
	return mockEnvironConfig()
}

func (s *mockStateAccess) EnvironUUID() string {
	s.AddCall(environUUIDCall)

	cfg, err := mockEnvironConfig()
	if err != nil {
		panic(err)
	}

	uuid, _ := cfg.UUID()
	return uuid
}

func mockEnvironConfig() (*config.Config, error) {
	mockCfg := dummy.SampleConfig().Merge(coretesting.Attrs{
		"type": "mock",
	})

	return config.New(config.NoDefaults, mockCfg)
}
