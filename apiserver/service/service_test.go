// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
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
				{Error: &params.Error{Message: `service "not-a-service" not found`, Code: "not found"}},
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
	_, err = service.ParseSettingsCompatible(ch, options)
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

func setupStoragePool(c *gc.C, st *state.State) {
	pm := poolmanager.New(state.NewStateSettings(st))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceDeployWithStorage(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, ch := s.UploadCharm(c, "utopic/storage-block-10", "storage-block")
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
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
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
			Pool:  "loop",
		},
	})
}

func (s *serviceSuite) TestServiceDeployWithInvalidStoragePool(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, _ := s.UploadCharm(c, "utopic/storage-block-0", "storage-block")
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
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `.* pool "foo" not found`)
}

func (s *serviceSuite) TestServiceDeployWithUnsupportedStoragePool(c *gc.C) {
	registry.RegisterProvider("hostloop", &mockStorageProvider{kind: storage.StorageKindBlock})
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("host-loop-pool", provider.HostLoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	curl, _ := s.UploadCharm(c, "utopic/storage-block-0", "storage-block")
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
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		`.*pool "host-loop-pool" uses storage provider "hostloop" which is not supported for models of type "dummy"`)
}

func (s *serviceSuite) TestServiceDeployDefaultFilesystemStorage(c *gc.C) {
	setupStoragePool(c, s.State)
	curl, ch := s.UploadCharm(c, "trusty/storage-filesystem-1", "storage-filesystem")
	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
	}
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestServiceDeploy(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"},
		},
	}
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	svc := apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, cons)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
}

func (s *serviceSuite) TestServiceDeployWithInvalidPlacement(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		NumUnits:    1,
		Constraints: cons,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "invalid"},
		},
	}
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.NotNil)
	c.Assert(results.Results[0].Error.Error(), gc.Matches, ".* invalid placement is invalid")
}

func (s *serviceSuite) testClientServicesDeployWithBindings(c *gc.C, endpointBindings, expected map[string]string) {
	curl, _ := s.UploadCharm(c, "utopic/riak-42", "riak")

	var cons constraints.Value
	args := params.ServiceDeploy{
		ServiceName:      "service",
		CharmUrl:         curl.String(),
		NumUnits:         1,
		Constraints:      cons,
		EndpointBindings: endpointBindings,
	}

	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	service, err := s.State.Service(args.ServiceName)
	c.Assert(err, jc.ErrorIsNil)

	retrievedBindings, err := service.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retrievedBindings, jc.DeepEquals, expected)
}

func (s *serviceSuite) TestClientServicesDeployWithBindings(c *gc.C) {
	s.State.AddSpace("a-space", "", nil, true)
	expected := map[string]string{
		"endpoint": "a-space",
		"ring":     "",
		"admin":    "",
	}
	endpointBindings := map[string]string{"endpoint": "a-space"}
	s.testClientServicesDeployWithBindings(c, endpointBindings, expected)
}

func (s *serviceSuite) TestClientServicesDeployWithDefaultBindings(c *gc.C) {
	expected := map[string]string{
		"endpoint": "",
		"ring":     "",
		"admin":    "",
	}
	s.testClientServicesDeployWithBindings(c, nil, expected)
}

// TODO(wallyworld) - the following charm tests have been moved from the apiserver/client
// package in order to use the fake charm store testing infrastructure. They are legacy tests
// written to use the api client instead of the apiserver logic. They need to be rewritten and
// feature tests added.

func (s *serviceSuite) TestAddCharm(c *gc.C) {
	var blobs blobs
	s.PatchValue(service.NewStateStorage, func(uuid string, session *mgo.Session) statestorage.Storage {
		storage := statestorage.NewStorage(uuid, session)
		return &recordingStorage{Storage: storage, blobs: &blobs}
	})

	client := s.APIState.Client()
	// First test the sanity checks.
	err := client.AddCharm(&charm.URL{Name: "nonsense"})
	c.Assert(err, gc.ErrorMatches, `cannot parse charm or bundle URL: ":nonsense-0"`)
	err = client.AddCharm(charm.MustParseURL("local:precise/dummy"))
	c.Assert(err, gc.ErrorMatches, "only charm store charm URLs are supported, with cs: schema")
	err = client.AddCharm(charm.MustParseURL("cs:precise/wordpress"))
	c.Assert(err, gc.ErrorMatches, "charm URL must include revision")

	// Add a charm, without uploading it to storage, to
	// check that AddCharm does not try to do it.
	charmDir := testcharms.Repo.CharmDir("dummy")
	ident := fmt.Sprintf("%s-%d", charmDir.Meta().Name, charmDir.Revision())
	curl := charm.MustParseURL("cs:quantal/" + ident)
	sch, err := s.State.AddCharm(charmDir, curl, "", ident+"-sha256")
	c.Assert(err, jc.ErrorIsNil)

	// AddCharm should see the charm in state and not upload it.
	err = client.AddCharm(sch.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(blobs.m, gc.HasLen, 0)

	// Now try adding another charm completely.
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = client.AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Verify it's in state and it got uploaded.
	storage := statestorage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	sch, err = s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

func (s *serviceSuite) TestAddCharmWithAuthorization(c *gc.C) {
	// Upload a new charm to the charm store.
	curl, _ := s.UploadCharm(c, "cs:~restricted/precise/wordpress-3", "wordpress")

	// Change permissions on the new charm such that only bob
	// can read from it.
	s.DischargeUser = "restricted"
	err := s.Client.Put("/"+curl.Path()+"/meta/perm/read", []string{"bob"})
	c.Assert(err, jc.ErrorIsNil)

	// Try to add a charm to the environment without authorization.
	s.DischargeUser = ""
	err = s.APIState.Client().AddCharm(curl)
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: discharge denied \(unauthorized access\)`)

	tryAs := func(user string) error {
		client := csclient.New(csclient.Params{
			URL: s.Srv.URL,
		})
		s.DischargeUser = user
		var m *macaroon.Macaroon
		err = client.Get("/delegatable-macaroon", &m)
		c.Assert(err, gc.IsNil)

		return service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	}
	// Try again with authorization for the wrong user.
	err = tryAs("joe")
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: unauthorized: access denied for user "joe"`)

	// Try again with the correct authorization this time.
	err = tryAs("bob")
	c.Assert(err, gc.IsNil)

	// Verify that it has actually been uploaded.
	_, err = s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
}

func (s *serviceSuite) TestAddCharmConcurrently(c *gc.C) {
	var putBarrier sync.WaitGroup
	var blobs blobs
	s.PatchValue(service.NewStateStorage, func(uuid string, session *mgo.Session) statestorage.Storage {
		storage := statestorage.NewStorage(uuid, session)
		return &recordingStorage{Storage: storage, blobs: &blobs, putBarrier: &putBarrier}
	})

	client := s.APIState.Client()
	curl, _ := s.UploadCharm(c, "trusty/wordpress-3", "wordpress")

	// Try adding the same charm concurrently from multiple goroutines
	// to test no "duplicate key errors" are reported (see lp bug
	// #1067979) and also at the end only one charm document is
	// created.

	var wg sync.WaitGroup
	// We don't add them 1-by-1 because that would allow each goroutine to
	// finish separately without actually synchronizing between them
	putBarrier.Add(10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			c.Assert(client.AddCharm(curl), gc.IsNil, gc.Commentf("goroutine %d", index))
			sch, err := s.State.Charm(curl)
			c.Assert(err, gc.IsNil, gc.Commentf("goroutine %d", index))
			c.Assert(sch.URL(), jc.DeepEquals, curl, gc.Commentf("goroutine %d", index))
		}(i)
	}
	wg.Wait()

	blobs.Lock()

	c.Assert(blobs.m, gc.HasLen, 10)

	// Verify there is only a single uploaded charm remains and it
	// contains the correct data.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	storagePath := sch.StoragePath()
	c.Assert(blobs.m[storagePath], jc.IsTrue)
	for path, exists := range blobs.m {
		if path != storagePath {
			c.Assert(exists, jc.IsFalse)
		}
	}

	storage := statestorage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

func (s *serviceSuite) assertUploaded(c *gc.C, storage statestorage.Storage, storagePath, expectedSHA256 string) {
	reader, _, err := storage.Get(storagePath)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *serviceSuite) TestAddCharmOverwritesPlaceholders(c *gc.C) {
	client := s.APIState.Client()
	curl, _ := s.UploadCharm(c, "trusty/wordpress-42", "wordpress")

	// Add a placeholder with the same charm URL.
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Now try to add the charm, which will convert the placeholder to
	// a pending charm.
	err = client.AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the document's flags were reset as expected.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), jc.DeepEquals, curl)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *serviceSuite) TestServiceGetCharmURL(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	result, err := s.serviceApi.GetCharmURL(params.ServiceGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "local:quantal/wordpress-3")
}

func (s *serviceSuite) TestServiceSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsFalse)
}

func (s *serviceSuite) setupServiceSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) assertServiceSetCharm(c *gc.C, forceUnits bool) {
	err := s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    "cs:~who/precise/wordpress-3",
		ForceUnits:  forceUnits,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:~who/precise/wordpress-3")
}

func (s *serviceSuite) assertServiceSetCharmBlocked(c *gc.C, msg string) {
	err := s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    "cs:~who/precise/wordpress-3",
	})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroyServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockDestroyModel(c, "TestBlockDestroyServiceSetCharm")
	s.assertServiceSetCharm(c, false)
}

func (s *serviceSuite) TestBlockRemoveServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceSetCharm")
	s.assertServiceSetCharm(c, false)
}

func (s *serviceSuite) TestBlockChangesServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceSetCharm")
	s.assertServiceSetCharmBlocked(c, "TestBlockChangesServiceSetCharm")
}

func (s *serviceSuite) TestServiceSetCharmForceUnits(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		ForceUnits:  true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsTrue)
}

func (s *serviceSuite) TestBlockServiceSetCharmForce(c *gc.C) {
	s.setupServiceSetCharm(c)

	// block all changes
	s.BlockAllChanges(c, "TestBlockServiceSetCharmForce")
	s.BlockRemoveObject(c, "TestBlockServiceSetCharmForce")
	s.BlockDestroyModel(c, "TestBlockServiceSetCharmForce")

	s.assertServiceSetCharm(c, true)
}

func (s *serviceSuite) TestServiceSetCharmInvalidService(c *gc.C) {
	err := s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "badservice",
		CharmUrl:    "cs:precise/wordpress-3",
		ForceSeries: true,
		ForceUnits:  true,
	})
	c.Assert(err, gc.ErrorMatches, `service "badservice" not found`)
}

func (s *serviceSuite) TestServiceAddCharmErrors(c *gc.C) {
	for url, expect := range map[string]string{
		"wordpress":                   "charm URL must include revision",
		"cs:wordpress":                "charm URL must include revision",
		"cs:precise/wordpress":        "charm URL must include revision",
		"cs:precise/wordpress-999999": `cannot retrieve "cs:precise/wordpress-999999": charm not found`,
	} {
		c.Logf("test %s", url)
		err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{
			URL: url,
		})
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func (s *serviceSuite) TestServiceSetCharmLegacy(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	// Even with forceSeries = true, we can't change a charm where
	// the series is sepcified in the URL.
	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		ForceSeries: true,
	})
	c.Assert(err, gc.ErrorMatches, "cannot change a service's series")
}

func (s *serviceSuite) TestServiceSetCharmUnsupportedSeries(c *gc.C) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			Series:      "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series2")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
	})
	c.Assert(err, gc.ErrorMatches, "cannot upgrade charm, only these series are supported: trusty, wily")
}

func (s *serviceSuite) TestServiceSetCharmUnsupportedSeriesForce(c *gc.C) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			Series:      "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharmMultiSeries(c, "~who/multi-series2", "multi-series2")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		ForceSeries: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	svc, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:~who/multi-series2-0")
}

func (s *serviceSuite) TestServiceSetCharmWrongOS(c *gc.C) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			Series:      "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharmMultiSeries(c, "~who/multi-series-windows", "multi-series-windows")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		ForceSeries: true,
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade charm, OS "Ubuntu" not supported by charm`)
}

type testModeCharmRepo struct {
	*charmrepo.CharmStore
	testMode bool
}

// WithTestMode returns a repository Interface where test mode is enabled.
func (s *testModeCharmRepo) WithTestMode() charmrepo.Interface {
	s.testMode = true
	return s.CharmStore.WithTestMode()
}

func (s *serviceSuite) TestSpecializeStoreOnDeployServiceSetCharmAndAddCharm(c *gc.C) {
	repo := &testModeCharmRepo{}
	s.PatchValue(&service.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL
		repo.CharmStore = charmrepo.NewCharmStore(p)
		return repo
	})
	attrs := map[string]interface{}{"test-mode": true}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the store's test mode is enabled when calling service Deploy.
	curl, _ := s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling SetCharm.
	curl, _ = s.UploadCharm(c, "trusty/wordpress-2", "wordpress")
	err = s.serviceApi.SetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
	})
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling AddCharm.
	curl, _ = s.UploadCharm(c, "utopic/riak-42", "riak")
	err = s.APIState.Client().AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo.testMode, jc.IsTrue)
}

func (s *serviceSuite) setupServiceDeploy(c *gc.C, args string) (*charm.URL, charm.Charm, constraints.Value) {
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse(args)
	return curl, ch, cons
}

func (s *serviceSuite) assertServiceDeployPrincipal(c *gc.C, curl *charm.URL, ch charm.Charm, mem4g constraints.Value) {
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
			Constraints: mem4g,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, mem4g)
}

func (s *serviceSuite) assertServiceDeployPrincipalBlocked(c *gc.C, msg string, curl *charm.URL, mem4g constraints.Value) {
	_, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
			Constraints: mem4g,
		}}})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroyServiceDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockDestroyModel(c, "TestBlockDestroyServiceDeployPrincipal")
	s.assertServiceDeployPrincipal(c, curl, bundle, cons)
}

func (s *serviceSuite) TestBlockRemoveServiceDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockRemoveObject(c, "TestBlockRemoveServiceDeployPrincipal")
	s.assertServiceDeployPrincipal(c, curl, bundle, cons)
}

func (s *serviceSuite) TestBlockChangesServiceDeployPrincipal(c *gc.C) {
	curl, _, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockAllChanges(c, "TestBlockChangesServiceDeployPrincipal")
	s.assertServiceDeployPrincipalBlocked(c, "TestBlockChangesServiceDeployPrincipal", curl, cons)
}

func (s *serviceSuite) TestServiceDeploySubordinate(c *gc.C) {
	curl, ch := s.UploadCharm(c, "utopic/logging-47", "logging")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service-name",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *serviceSuite) TestServiceDeployConfig(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service-name",
			NumUnits:    1,
			ConfigYAML:  "service-name:\n  username: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"username": "fred"})
}

func (s *serviceSuite) TestServiceDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service-name",
			NumUnits:    1,
			ConfigYAML:  "service-name:\n  skill-level: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceSuite) TestServiceDeployToMachine(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service-name",
			NumUnits:    1,
			ConfigYAML:  "service-name:\n  username: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("service-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *serviceSuite) TestServiceDeployToMachineNotFound(c *gc.C) {
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    "cs:precise/service-name-1",
			ServiceName: "service-name",
			NumUnits:    1,
			Placement:   []*instance.Placement{instance.MustParsePlacement("42")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `cannot deploy "service-name" to machine 42: machine 42 not found`)

	_, err = s.State.Service("service-name")
	c.Assert(err, gc.ErrorMatches, `service "service-name" not found`)
}

func (s *serviceSuite) TestServiceDeployServiceOwner(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service.GetOwnerTag(), gc.Equals, s.authorizer.GetAuthTag().String())
}

func (s *serviceSuite) deployServiceForUpdateTests(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-1", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.Deploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    1,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *serviceSuite) checkClientServiceUpdateSetCharm(c *gc.C, forceCharmUrl bool) {
	s.deployServiceForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      curl.String(),
		ForceCharmUrl: forceCharmUrl,
	}
	err = s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl.String())
	c.Assert(force, gc.Equals, forceCharmUrl)
}

func (s *serviceSuite) TestServiceUpdateSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *serviceSuite) TestBlockDestroyServiceUpdate(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyServiceUpdate")
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *serviceSuite) TestBlockRemoveServiceUpdate(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveServiceUpdate")
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *serviceSuite) setupServiceUpdate(c *gc.C) string {
	s.deployServiceForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	return curl.String()
}

func (s *serviceSuite) TestBlockChangeServiceUpdate(c *gc.C) {
	curl := s.setupServiceUpdate(c)
	s.BlockAllChanges(c, "TestBlockChangeServiceUpdate")
	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      curl,
		ForceCharmUrl: false,
	}
	err := s.serviceApi.Update(args)
	s.AssertBlocked(c, err, "TestBlockChangeServiceUpdate")
}

func (s *serviceSuite) TestServiceUpdateForceSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, true)
}

func (s *serviceSuite) TestBlockServiceUpdateForced(c *gc.C) {
	curl := s.setupServiceUpdate(c)

	// block all changes. Force should ignore block :)
	s.BlockAllChanges(c, "TestBlockServiceUpdateForced")
	s.BlockDestroyModel(c, "TestBlockServiceUpdateForced")
	s.BlockRemoveObject(c, "TestBlockServiceUpdateForced")

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      curl,
		ForceCharmUrl: true,
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl)
	c.Assert(force, jc.IsTrue)
}

func (s *serviceSuite) TestServiceUpdateSetCharmNotFound(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	args := params.ServiceUpdate{
		ServiceName: "wordpress",
		CharmUrl:    "cs:precise/wordpress-999999",
	}
	err := s.serviceApi.Update(args)
	c.Check(err, gc.ErrorMatches, `charm "cs:precise/wordpress-999999" not found`)
}

func (s *serviceSuite) TestServiceUpdateSetMinUnits(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set minimum units for the service.
	minUnits := 2
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, minUnits)
}

func (s *serviceSuite) TestServiceUpdateSetMinUnitsError(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set a negative minimum number of units for the service.
	minUnits := -1
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for service "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, 0)
}

func (s *serviceSuite) TestServiceUpdateSetSettingsStrings(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:     "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestServiceUpdateSetSettingsYAML(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "dummy:\n  title: y-title\n  username: y-user",
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestClientServiceUpdateSetSettingsGetYAML(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "charm: dummy\nservice: dummy\nsettings:\n  title:\n    value: y-title\n    type: string\n  username:\n    value: y-user\n  ignore:\n    blah: true",
	}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestServiceUpdateSetConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		Constraints: &cons,
	}
	err = s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *serviceSuite) TestServiceUpdateAllParams(c *gc.C) {
	s.deployServiceForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	// Update all the service attributes.
	minUnits := 3
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ServiceUpdate{
		ServiceName:     "service",
		CharmUrl:        curl.String(),
		ForceCharmUrl:   true,
		MinUnits:        &minUnits,
		SettingsStrings: map[string]string{"blog-title": "string-title"},
		SettingsYAML:    "service:\n  blog-title: yaml-title\n",
		Constraints:     &cons,
	}
	err = s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the service has been correctly updated.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)

	// Check the charm.
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsTrue)

	// Check the minimum number of units.
	c.Assert(service.MinUnits(), gc.Equals, minUnits)

	// Check the settings: also ensure the YAML settings take precedence
	// over strings ones.
	expectedSettings := charm.Settings{"blog-title": "yaml-title"}
	obtainedSettings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedSettings, gc.DeepEquals, expectedSettings)

	// Check the constraints.
	obtainedConstraints, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedConstraints, gc.DeepEquals, cons)
}

func (s *serviceSuite) TestServiceUpdateNoParams(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Calling Update with no parameters set is a no-op.
	args := params.ServiceUpdate{ServiceName: "wordpress"}
	err := s.serviceApi.Update(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceUpdateNoService(c *gc.C) {
	err := s.serviceApi.Update(params.ServiceUpdate{})
	c.Assert(err, gc.ErrorMatches, `"" is not a valid service name`)
}

func (s *serviceSuite) TestServiceUpdateInvalidService(c *gc.C) {
	args := params.ServiceUpdate{ServiceName: "no-such-service"}
	err := s.serviceApi.Update(args)
	c.Assert(err, gc.ErrorMatches, `service "no-such-service" not found`)
}

var (
	validSetTestValue = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
)

func (s *serviceSuite) TestServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.serviceApi.Set(params.ServiceSet{ServiceName: "dummy", Options: map[string]string{
		"title":    "foobar",
		"username": validSetTestValue,
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	})

	err = s.serviceApi.Set(params.ServiceSet{ServiceName: "dummy", Options: map[string]string{
		"title":    "barfoo",
		"username": "",
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "barfoo",
		"username": "",
	})
}

func (s *serviceSuite) assertServiceSetBlocked(c *gc.C, dummy *state.Service, msg string) {
	err := s.serviceApi.Set(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) assertServiceSet(c *gc.C, dummy *state.Service) {
	err := s.serviceApi.Set(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	})
}

func (s *serviceSuite) TestBlockDestroyServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyModel(c, "TestBlockDestroyServiceSet")
	s.assertServiceSet(c, dummy)
}

func (s *serviceSuite) TestBlockRemoveServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveServiceSet")
	s.assertServiceSet(c, dummy)
}

func (s *serviceSuite) TestBlockChangesServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangesServiceSet")
	s.assertServiceSetBlocked(c, dummy, "TestBlockChangesServiceSet")
}

func (s *serviceSuite) TestServerUnset(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.serviceApi.Set(params.ServiceSet{ServiceName: "dummy", Options: map[string]string{
		"title":    "foobar",
		"username": "user name",
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.serviceApi.Unset(params.ServiceUnset{ServiceName: "dummy", Options: []string{"username"}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "foobar",
	})
}

func (s *serviceSuite) setupServerUnsetBlocked(c *gc.C) *state.Service {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.serviceApi.Set(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": "user name",
		}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})
	return dummy
}

func (s *serviceSuite) assertServerUnset(c *gc.C, dummy *state.Service) {
	err := s.serviceApi.Unset(params.ServiceUnset{
		ServiceName: "dummy",
		Options:     []string{"username"},
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "foobar",
	})
}

func (s *serviceSuite) assertServerUnsetBlocked(c *gc.C, dummy *state.Service, msg string) {
	err := s.serviceApi.Unset(params.ServiceUnset{
		ServiceName: "dummy",
		Options:     []string{"username"},
	})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroyServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockDestroyModel(c, "TestBlockDestroyServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *serviceSuite) TestBlockRemoveServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *serviceSuite) TestBlockChangesServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockAllChanges(c, "TestBlockChangesServerUnset")
	s.assertServerUnsetBlocked(c, dummy, "TestBlockChangesServerUnset")
}

var clientAddServiceUnitsTests = []struct {
	about    string
	service  string // if not set, defaults to 'dummy'
	expected []string
	to       string
	err      string
}{
	{
		about:    "returns unit names",
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
}

func (s *serviceSuite) TestClientAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	for i, t := range clientAddServiceUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		serviceName := t.service
		if serviceName == "" {
			serviceName = "dummy"
		}
		args := params.AddServiceUnits{
			ServiceName: serviceName,
			NumUnits:    len(t.expected),
		}
		if t.to != "" {
			args.Placement = []*instance.Placement{instance.MustParsePlacement(t.to)}
		}
		result, err := s.serviceApi.AddUnits(args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/3")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *serviceSuite) TestAddServiceUnitsToNewContainer(c *gc.C) {
	svc := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.serviceApi.AddUnits(params.AddServiceUnits{
		ServiceName: "dummy",
		NumUnits:    1,
		Placement:   []*instance.Placement{instance.MustParsePlacement("lxc:" + machine.Id())},
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id()+"/lxc/0")
}

var addServiceUnitTests = []struct {
	about      string
	service    string // if not set, defaults to 'dummy'
	expected   []string
	machineIds []string
	placement  []*instance.Placement
	err        string
}{
	{
		about:      "valid placement directives",
		expected:   []string{"dummy/0"},
		placement:  []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"}},
		machineIds: []string{"1"},
	}, {
		about:      "direct machine assignment placement directive",
		expected:   []string{"dummy/1", "dummy/2"},
		placement:  []*instance.Placement{{"#", "1"}, {"lxc", "1"}},
		machineIds: []string{"1", "1/lxc/0"},
	}, {
		about:     "invalid placement directive",
		err:       ".* invalid placement is invalid",
		expected:  []string{"dummy/3"},
		placement: []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "invalid"}},
	},
}

func (s *serviceSuite) TestAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Add a machine for the units to be placed on.
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	for i, t := range addServiceUnitTests {
		c.Logf("test %d. %s", i, t.about)
		serviceName := t.service
		if serviceName == "" {
			serviceName = "dummy"
		}
		result, err := s.serviceApi.AddUnits(params.AddServiceUnits{
			ServiceName: serviceName,
			NumUnits:    len(t.expected),
			Placement:   t.placement,
		})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
		for i, unitName := range result.Units {
			u, err := s.BackingState.Unit(unitName)
			c.Assert(err, jc.ErrorIsNil)
			assignedMachine, err := u.AssignedMachineId()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(assignedMachine, gc.Equals, t.machineIds[i])
		}
	}
}

func (s *serviceSuite) assertAddServiceUnits(c *gc.C) {
	result, err := s.serviceApi.AddUnits(params.AddServiceUnits{
		ServiceName: "dummy",
		NumUnits:    3,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Units, gc.DeepEquals, []string{"dummy/0", "dummy/1", "dummy/2"})

	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/0")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *serviceSuite) TestServiceCharmRelations(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.serviceApi.CharmRelations(params.ServiceCharmRelations{"blah"})
	c.Assert(err, gc.ErrorMatches, `service "blah" not found`)

	result, err := s.serviceApi.CharmRelations(params.ServiceCharmRelations{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CharmRelations, gc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *serviceSuite) assertAddServiceUnitsBlocked(c *gc.C, msg string) {
	_, err := s.serviceApi.AddUnits(params.AddServiceUnits{
		ServiceName: "dummy",
		NumUnits:    3,
	})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroyAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyModel(c, "TestBlockDestroyAddServiceUnits")
	s.assertAddServiceUnits(c)
}

func (s *serviceSuite) TestBlockRemoveAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveAddServiceUnits")
	s.assertAddServiceUnits(c)
}

func (s *serviceSuite) TestBlockChangeAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangeAddServiceUnits")
	s.assertAddServiceUnitsBlocked(c, "TestBlockChangeAddServiceUnits")
}

func (s *serviceSuite) TestAddUnitToMachineNotFound(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	_, err := s.serviceApi.AddUnits(params.AddServiceUnits{
		ServiceName: "dummy",
		NumUnits:    3,
		Placement:   []*instance.Placement{instance.MustParsePlacement("42")},
	})
	c.Assert(err, gc.ErrorMatches, `adding new machine to host unit "dummy/0": machine 42 not found`)
}

func (s *serviceSuite) TestServiceExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	serviceNames := []string{"dummy-service", "exposed-service"}
	svcs := make([]*state.Service, len(serviceNames))
	var err error
	for i, name := range serviceNames {
		svcs[i] = s.AddTestingService(c, name, charm)
		c.Assert(svcs[i].IsExposed(), jc.IsFalse)
	}
	err = svcs[1].SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svcs[1].IsExposed(), jc.IsTrue)
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err = s.serviceApi.Expose(params.ServiceExpose{t.service})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			service, err := s.State.Service(t.service)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(service.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

func (s *serviceSuite) setupServiceExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	serviceNames := []string{"dummy-service", "exposed-service"}
	svcs := make([]*state.Service, len(serviceNames))
	var err error
	for i, name := range serviceNames {
		svcs[i] = s.AddTestingService(c, name, charm)
		c.Assert(svcs[i].IsExposed(), jc.IsFalse)
	}
	err = svcs[1].SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svcs[1].IsExposed(), jc.IsTrue)
}

var serviceExposeTests = []struct {
	about   string
	service string
	err     string
	exposed bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "expose a service",
		service: "dummy-service",
		exposed: true,
	},
	{
		about:   "expose an already exposed service",
		service: "exposed-service",
		exposed: true,
	},
}

func (s *serviceSuite) assertServiceExpose(c *gc.C) {
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.serviceApi.Expose(params.ServiceExpose{t.service})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			service, err := s.State.Service(t.service)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(service.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

func (s *serviceSuite) assertServiceExposeBlocked(c *gc.C, msg string) {
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.serviceApi.Expose(params.ServiceExpose{t.service})
		s.AssertBlocked(c, err, msg)
	}
}

func (s *serviceSuite) TestBlockDestroyServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyServiceExpose")
	s.assertServiceExpose(c)
}

func (s *serviceSuite) TestBlockRemoveServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceExpose")
	s.assertServiceExpose(c)
}

func (s *serviceSuite) TestBlockChangesServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceExpose")
	s.assertServiceExposeBlocked(c, "TestBlockChangesServiceExpose")
}

var serviceUnexposeTests = []struct {
	about    string
	service  string
	err      string
	initial  bool
	expected bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:    "unexpose a service",
		service:  "dummy-service",
		initial:  true,
		expected: false,
	},
	{
		about:    "unexpose an already unexposed service",
		service:  "dummy-service",
		initial:  false,
		expected: false,
	},
}

func (s *serviceSuite) TestServiceUnexpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		svc := s.AddTestingService(c, "dummy-service", charm)
		if t.initial {
			svc.SetExposed()
		}
		c.Assert(svc.IsExposed(), gc.Equals, t.initial)
		err := s.serviceApi.Unexpose(params.ServiceUnexpose{t.service})
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			svc.Refresh()
			c.Assert(svc.IsExposed(), gc.Equals, t.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		err = svc.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serviceSuite) setupServiceUnexpose(c *gc.C) *state.Service {
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", charm)
	svc.SetExposed()
	c.Assert(svc.IsExposed(), gc.Equals, true)
	return svc
}

func (s *serviceSuite) assertServiceUnexpose(c *gc.C, svc *state.Service) {
	err := s.serviceApi.Unexpose(params.ServiceUnexpose{"dummy-service"})
	c.Assert(err, jc.ErrorIsNil)
	svc.Refresh()
	c.Assert(svc.IsExposed(), gc.Equals, false)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) assertServiceUnexposeBlocked(c *gc.C, svc *state.Service, msg string) {
	err := s.serviceApi.Unexpose(params.ServiceUnexpose{"dummy-service"})
	s.AssertBlocked(c, err, msg)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestBlockDestroyServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyServiceUnexpose")
	s.assertServiceUnexpose(c, svc)
}

func (s *serviceSuite) TestBlockRemoveServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceUnexpose")
	s.assertServiceUnexpose(c, svc)
}

func (s *serviceSuite) TestBlockChangesServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceUnexpose")
	s.assertServiceUnexposeBlocked(c, svc, "TestBlockChangesServiceUnexpose")
}

var serviceDestroyTests = []struct {
	about   string
	service string
	err     string
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "destroy a service",
		service: "dummy-service",
	},
	{
		about:   "destroy an already destroyed service",
		service: "dummy-service",
		err:     `service "dummy-service" not found`,
	},
}

func (s *serviceSuite) TestServiceDestroy(c *gc.C) {
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))
	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.serviceApi.Destroy(params.ServiceDestroy{t.service})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	// Now do Destroy on a service with units. Destroy will
	// cause the service to be not-Alive, but will not remove its
	// document.
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceApi.Destroy(params.ServiceDestroy{serviceName})
	c.Assert(err, jc.ErrorIsNil)
	err = service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func (s *serviceSuite) TestBlockServiceDestroy(c *gc.C) {
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))

	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockServiceDestroy")
	err := s.serviceApi.Destroy(params.ServiceDestroy{"dummy-service"})
	s.AssertBlocked(c, err, "TestBlockServiceDestroy")
	// Tests may have invalid service names.
	service, err := s.State.Service("dummy-service")
	if err == nil {
		// For valid service names, check that service is alive :-)
		assertLife(c, service, state.Alive)
	}
}

func (s *serviceSuite) TestDestroyPrincipalUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetAgentStatus(state.StatusIdle, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDestroyPrincipalUnits(c, units)
}

func (s *serviceSuite) TestDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *serviceSuite) assertDestroyPrincipalUnits(c *gc.C, units []*state.Unit) {
	// Destroy 2 of them; check they become Dying.
	err := s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/2", "wordpress/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[2], state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"boojum/123", "wordpress/3"},
	})
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	assertLife(c, units[3], state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wp0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "wordpress/4"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dead)
	assertLife(c, units[4], state.Dying)
}

func (s *serviceSuite) setupDestroyPrincipalUnits(c *gc.C) []*state.Unit {
	units := make([]*state.Unit, 5)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetAgentStatus(state.StatusIdle, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	return units
}

func (s *serviceSuite) assertBlockedErrorAndLiveliness(
	c *gc.C,
	err error,
	msg string,
	living1 state.Living,
	living2 state.Living,
	living3 state.Living,
	living4 state.Living,
) {
	s.AssertBlocked(c, err, msg)
	assertLife(c, living1, state.Alive)
	assertLife(c, living2, state.Alive)
	assertLife(c, living3, state.Alive)
	assertLife(c, living4, state.Alive)
}

func (s *serviceSuite) TestBlockChangesDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyPrincipalUnits")
	err := s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *serviceSuite) TestBlockRemoveDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyPrincipalUnits")
	err := s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *serviceSuite) TestBlockDestroyDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockDestroyModel(c, "TestBlockDestroyDestroyPrincipalUnits")
	err := s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)
}

func (s *serviceSuite) assertDestroySubordinateUnits(c *gc.C, wordpress0, logging0 *state.Unit) {
	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be responsible for destroying the subordinate.)
	err := s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, wordpress0, state.Dying)
	assertLife(c, logging0, state.Alive)
}

func (s *serviceSuite) TestBlockRemoveDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockRemoveObject(c, "TestBlockRemoveDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *serviceSuite) TestBlockChangesDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockAllChanges(c, "TestBlockChangesDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *serviceSuite) TestBlockDestroyDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.serviceApi.DestroyUnits(params.DestroyServiceUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *serviceSuite) TestClientSetServiceConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.serviceApi.SetConstraints(params.SetConstraints{ServiceName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *serviceSuite) setupSetServiceConstraints(c *gc.C) (*state.Service, constraints.Value) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	return service, cons
}

func (s *serviceSuite) assertSetServiceConstraints(c *gc.C, service *state.Service, cons constraints.Value) {
	err := s.serviceApi.SetConstraints(params.SetConstraints{ServiceName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *serviceSuite) assertSetServiceConstraintsBlocked(c *gc.C, msg string, service *state.Service, cons constraints.Value) {
	err := s.serviceApi.SetConstraints(params.SetConstraints{ServiceName: "dummy", Constraints: cons})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroySetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockDestroyModel(c, "TestBlockDestroySetServiceConstraints")
	s.assertSetServiceConstraints(c, svc, cons)
}

func (s *serviceSuite) TestBlockRemoveSetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockRemoveObject(c, "TestBlockRemoveSetServiceConstraints")
	s.assertSetServiceConstraints(c, svc, cons)
}

func (s *serviceSuite) TestBlockChangesSetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockAllChanges(c, "TestBlockChangesSetServiceConstraints")
	s.assertSetServiceConstraintsBlocked(c, "TestBlockChangesSetServiceConstraints", svc, cons)
}

func (s *serviceSuite) TestClientGetServiceConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Check we can get the constraints.
	result, err := s.serviceApi.GetConstraints(params.GetServiceConstraints{"dummy"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Constraints, gc.DeepEquals, cons)
}

func (s *serviceSuite) checkEndpoints(c *gc.C, endpoints map[string]charm.Relation) {
	c.Assert(endpoints["wordpress"], gc.DeepEquals, charm.Relation{
		Name:      "db",
		Role:      charm.RelationRole("requirer"),
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     charm.RelationScope("global"),
	})
	c.Assert(endpoints["mysql"], gc.DeepEquals, charm.Relation{
		Name:      "server",
		Role:      charm.RelationRole("provider"),
		Interface: "mysql",
		Optional:  false,
		Limit:     0,
		Scope:     charm.RelationScope("global"),
	})
}

func (s *serviceSuite) setupRelationScenario(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) assertAddRelation(c *gc.C, endpoints []string) {
	s.setupRelationScenario(c)
	res, err := s.serviceApi.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, jc.ErrorIsNil)
	s.checkEndpoints(c, res.Endpoints)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rels, err := wpSvc.Relations()
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), gc.Equals, 2)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, jc.ErrorIsNil)
	rels, err = mySvc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *serviceSuite) TestSuccessfullyAddRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints)
}

func (s *serviceSuite) TestBlockDestroyAddRelation(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"})
}
func (s *serviceSuite) TestBlockRemoveAddRelation(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"})
}

func (s *serviceSuite) TestBlockChangesAddRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.BlockAllChanges(c, "TestBlockChangesAddRelation")
	_, err := s.serviceApi.AddRelation(params.AddRelation{Endpoints: []string{"wordpress", "mysql"}})
	s.AssertBlocked(c, err, "TestBlockChangesAddRelation")
}

func (s *serviceSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints)
}

func (s *serviceSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress"}
	_, err := s.serviceApi.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *serviceSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.serviceApi.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *serviceSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Add a relation between wordpress and mysql.
	endpoints := []string{"wordpress", "mysql"}
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	// And try to add it again.
	_, err = s.serviceApi.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}

func (s *serviceSuite) setupDestroyRelationScenario(c *gc.C, endpoints []string) *state.Relation {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Add a relation between the endpoints.
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return relation
}

func (s *serviceSuite) assertDestroyRelation(c *gc.C, endpoints []string) {
	s.assertDestroyRelationSuccess(
		c,
		s.setupDestroyRelationScenario(c, endpoints),
		endpoints)
}

func (s *serviceSuite) assertDestroyRelationSuccess(c *gc.C, relation *state.Relation, endpoints []string) {
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, jc.ErrorIsNil)
	// Show that the relation was removed.
	c.Assert(relation.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *serviceSuite) TestSuccessfulDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *serviceSuite) TestSuccessfullyDestroyRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the DestroyRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *serviceSuite) TestNoRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress", "mysql"}
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *serviceSuite) TestAttemptDestroyingNonExistentRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	endpoints := []string{"riak", "wordpress"}
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *serviceSuite) TestAttemptDestroyingWithOnlyOneEndpoint(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress"}
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *serviceSuite) TestAttemptDestroyingPeerRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))

	endpoints := []string{"riak:ring"}
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
}

func (s *serviceSuite) TestAttemptDestroyingAlreadyDestroyedRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := []string{"wordpress", "mysql"}
	err = s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFound)

	// And try to destroy it again.
	err = s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *serviceSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	relation := s.setupDestroyRelationScenario(c, endpoints)
	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyRelation")
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyRelation")
	assertLife(c, relation, state.Alive)
}

func (s *serviceSuite) TestBlockChangeDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	relation := s.setupDestroyRelationScenario(c, endpoints)
	s.BlockAllChanges(c, "TestBlockChangeDestroyRelation")
	err := s.serviceApi.DestroyRelation(params.DestroyRelation{Endpoints: endpoints})
	s.AssertBlocked(c, err, "TestBlockChangeDestroyRelation")
	assertLife(c, relation, state.Alive)
}

func (s *serviceSuite) TestBlockDestroyDestroyRelation(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyDestroyRelation")
	endpoints := []string{"wordpress", "mysql"}
	s.assertDestroyRelation(c, endpoints)
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

type blobs struct {
	sync.Mutex
	m map[string]bool // maps path to added (true), or deleted (false)
}

// Add adds a path to the list of known paths.
func (b *blobs) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = true
}

// Remove marks a path as deleted, even if it was not previously Added.
func (b *blobs) Remove(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = false
}

func (b *blobs) check() {
	if b.m == nil {
		b.m = make(map[string]bool)
	}
}

type recordingStorage struct {
	statestorage.Storage
	putBarrier *sync.WaitGroup
	blobs      *blobs
}

func (s *recordingStorage) Put(path string, r io.Reader, size int64) error {
	if s.putBarrier != nil {
		// This goroutine has gotten to Put() so mark it Done() and
		// wait for the other goroutines to get to this point.
		s.putBarrier.Done()
		s.putBarrier.Wait()
	}
	if err := s.Storage.Put(path, r, size); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Add(path)
	return nil
}

func (s *recordingStorage) Remove(path string) error {
	if err := s.Storage.Remove(path); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Remove(path)
	return nil
}
