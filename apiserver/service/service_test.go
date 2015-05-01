// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

type serviceSuite struct {
	jujutesting.JujuConnSuite
	apiservertesting.CharmStoreSuite
	commontesting.BlockHelper

	serviceApi *service.API
	service    *state.Service
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&serviceSuite{})

var _ service.Service = (*service.API)(nil)

func (s *serviceSuite) SetUpSuite(c *gc.C) {
	s.CharmStoreSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *serviceSuite) TearDownSuite(c *gc.C) {
	s.CharmStoreSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.CharmStoreSuite.Session = s.JujuConnSuite.Session
	s.CharmStoreSuite.SetUpTest(c)

	s.service = s.Factory.MakeService(c, nil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.serviceApi, err = service.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TearDownTest(c *gc.C) {
	s.CharmStoreSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *serviceSuite) TestSetMetricCredentials(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: charm,
	})
	tests := []struct {
		about   string
		args    params.ServiceMetricCredentials
		results params.ErrorResults
	}{
		{
			"test one argument and it passes",
			params.ServiceMetricCredentials{[]params.ServiceMetricCredential{{
				s.service.Name(),
				[]byte("creds 1234"),
			}}},
			params.ErrorResults{[]params.ErrorResult{{Error: nil}}},
		},
		{
			"test two arguments and both pass",
			params.ServiceMetricCredentials{[]params.ServiceMetricCredential{
				{
					s.service.Name(),
					[]byte("creds 1234"),
				},
				{
					wordpress.Name(),
					[]byte("creds 4567"),
				},
			}},
			params.ErrorResults{[]params.ErrorResult{
				{Error: nil},
				{Error: nil},
			}},
		},
		{
			"test two arguments and second one fails",
			params.ServiceMetricCredentials{[]params.ServiceMetricCredential{
				{
					s.service.Name(),
					[]byte("creds 1234"),
				},
				{
					"not-a-service",
					[]byte("creds 4567"),
				},
			}},
			params.ErrorResults{[]params.ErrorResult{
				{Error: nil},
				{Error: &params.Error{`service "not-a-service" not found`, "not found"}},
			}},
		},
	}
	for i, t := range tests {
		c.Logf("Running test %d %v", i, t.about)
		results, err := s.serviceApi.SetMetricCredentials(t.args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, len(t.results.Results))
		c.Assert(results, gc.DeepEquals, t.results)

		for i, a := range t.args.Creds {
			if t.results.Results[i].Error == nil {
				svc, err := s.State.Service(a.ServiceName)
				c.Assert(err, jc.ErrorIsNil)
				creds := svc.MetricCredentials()
				c.Assert(creds, gc.DeepEquals, a.MetricCredentials)
			}
		}
	}
}

func (s *serviceSuite) TestCompatibleSettingsParsing(c *gc.C) {
	// Test the exported settings parsing in a compatible way.
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	svc, err := s.State.Service("dummy")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "local:quantal/dummy-1")

	// Empty string will be returned as nil.
	options := map[string]string{
		"title":    "foobar",
		"username": "",
	}
	settings, err := service.ParseSettingsCompatible(ch, options)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": nil,
	})

	// Illegal settings lead to an error.
	options = map[string]string{
		"yummy": "didgeridoo",
	}
	settings, err = service.ParseSettingsCompatible(ch, options)
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

func setupStoragePool(c *gc.C, st *state.State) {
	pm := poolmanager.New(state.NewStateSettings(st))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateEnvironConfig(map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) uploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	id := charm.MustParseReference(url)
	promulgated := false
	if id.User == "" {
		id.User = "who"
		promulgated = true
	}
	ch := testcharms.Repo.CharmArchive(c.MkDir(), name)
	id = s.Srv.UploadCharm(c, ch, id, promulgated)
	curl := (*charm.URL)(id)
	err := s.APIState.Client().AddCharmWithAuthorization(curl, nil)
	c.Assert(err, jc.ErrorIsNil)
	return curl, ch
}

func (s *serviceSuite) TestClientServiceDeployWithStorage(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, ch := s.uploadCharm(c, "utopic/storage-block-10", "storage-block")
	storageConstraints := map[string]storage.Constraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "loop-pool",
		},
	}

	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
		Storage:     storageConstraints,
	}
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	svc := apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, cons)
	storageConstraintsOut, err := svc.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageConstraintsOut, gc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "loop-pool",
		},
		"allecto": {
			Count: 0,
			Size:  1024,
			Pool:  "loop-pool",
		},
	})
}

func (s *serviceSuite) TestClientServiceDeployWithInvalidStoragePool(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, _ := s.uploadCharm(c, "utopic/storage-block-0", "storage-block")
	storageConstraints := map[string]storage.Constraints{
		"data": storage.Constraints{
			Pool:  "foo",
			Count: 1,
			Size:  1024,
		},
	}

	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
		Storage:     storageConstraints,
	}
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `.* pool "foo" not found`)
}

func (s *serviceSuite) TestClientServiceDeployWithUnsupportedStoragePool(c *gc.C) {
	registry.RegisterProvider("hostloop", &mockStorageProvider{kind: storage.StorageKindBlock})
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("host-loop-pool", provider.HostLoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	curl, _ := s.uploadCharm(c, "utopic/storage-block-0", "storage-block")
	storageConstraints := map[string]storage.Constraints{
		"data": storage.Constraints{
			Pool:  "host-loop-pool",
			Count: 1,
			Size:  1024,
		},
	}

	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
		Storage:     storageConstraints,
	}
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		`.*pool "host-loop-pool" uses storage provider "hostloop" which is not supported for environments of type "dummy"`)
}

func (s *serviceSuite) TestClientServiceDeployDefaultFilesystemStorage(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, ch := s.uploadCharm(c, "trusty/storage-filesystem-1", "storage-filesystem")
	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
	}
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	svc := apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, cons)
	storageConstraintsOut, err := svc.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageConstraintsOut, gc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "rootfs",
		},
	})
}

type mockStorageProvider struct {
	storage.Provider
	kind storage.StorageKind
}

func (m *mockStorageProvider) Scope() storage.Scope {
	return storage.ScopeMachine
}

func (m *mockStorageProvider) Supports(k storage.StorageKind) bool {
	return k == m.kind
}

func (m *mockStorageProvider) ValidateConfig(*storage.Config) error {
	return nil
}
