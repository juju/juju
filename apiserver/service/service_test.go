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
	err = st.UpdateEnvironConfig(map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestClientServiceDeployWithStorage(c *gc.C) {
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
			Pool:  "loop",
		},
	})
}

func (s *serviceSuite) TestClientServiceDeployWithInvalidStoragePool(c *gc.C) {
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
	curl, ch := s.UploadCharm(c, "trusty/storage-filesystem-1", "storage-filesystem")
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

func (s *serviceSuite) TestClientServiceDeployWithPlacement(c *gc.C) {
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
		ToMachineSpec: "will be ignored",
	}
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestClientServiceDeployWithInvalidPlacement(c *gc.C) {
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
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.NotNil)
	c.Assert(results.Results[0].Error.Error(), gc.Matches, ".* invalid placement is invalid")
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
	storage := statestorage.NewStorage(s.State.EnvironUUID(), s.State.MongoSession())
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
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: discharge denied`)

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

	storage := statestorage.NewStorage(s.State.EnvironUUID(), s.State.MongoSession())
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
	result, err := s.serviceApi.ServiceGetCharmURL(params.ServiceGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "local:quantal/wordpress-3")
}

func (s *serviceSuite) TestClientServiceSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	err = s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
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
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) assertServiceSetCharm(c *gc.C, force bool) {
	err := s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    "cs:~who/precise/wordpress-3",
		Force:       force,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:~who/precise/wordpress-3")
}

func (s *serviceSuite) assertServiceSetCharmBlocked(c *gc.C, force bool, msg string) {
	err := s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    "cs:~who/precise/wordpress-3",
		Force:       force,
	})
	s.AssertBlocked(c, err, msg)
}

func (s *serviceSuite) TestBlockDestroyServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceSetCharm")
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
	s.assertServiceSetCharmBlocked(c, false, "TestBlockChangesServiceSetCharm")
}

func (s *serviceSuite) TestClientServiceSetCharmForce(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	err = s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
		ServiceName: "service",
		CharmUrl:    curl.String(),
		Force:       true,
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
	s.BlockDestroyEnvironment(c, "TestBlockServiceSetCharmForce")

	s.assertServiceSetCharm(c, true)
}

func (s *serviceSuite) TestClientServiceSetCharmInvalidService(c *gc.C) {
	err := s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
		ServiceName: "badservice",
		CharmUrl:    "cs:precise/wordpress-3",
		Force:       true,
	})
	c.Assert(err, gc.ErrorMatches, `service "badservice" not found`)
}

func (s *serviceSuite) TestClientServiceAddCharmErrors(c *gc.C) {
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

type testModeCharmRepo struct {
	*charmrepo.CharmStore
	testMode bool
}

// WithTestMode returns a repository Interface where test mode is enabled.
func (s *testModeCharmRepo) WithTestMode() charmrepo.Interface {
	s.testMode = true
	return s.CharmStore.WithTestMode()
}

func (s *serviceSuite) TestClientSpecializeStoreOnDeployServiceSetCharmAndAddCharm(c *gc.C) {
	repo := &testModeCharmRepo{}
	s.PatchValue(&service.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL
		repo.CharmStore = charmrepo.NewCharmStore(p)
		return repo
	})
	attrs := map[string]interface{}{"test-mode": true}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the store's test mode is enabled when calling ServiceDeploy.
	curl, _ := s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:    curl.String(),
			ServiceName: "service",
			NumUnits:    3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling ServiceSetCharm.
	curl, _ = s.UploadCharm(c, "trusty/wordpress-2", "wordpress")
	err = s.serviceApi.ServiceSetCharm(params.ServiceSetCharm{
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
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	_, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceDeployPrincipal")
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

func (s *serviceSuite) TestClientServiceDeploySubordinate(c *gc.C) {
	curl, ch := s.UploadCharm(c, "utopic/logging-47", "logging")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestClientServiceDeployConfig(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestClientServiceDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestClientServiceDeployToMachine(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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

func (s *serviceSuite) TestClientServiceDeployToMachineNotFound(c *gc.C) {
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			CharmUrl:      "cs:precise/service-name-1",
			ServiceName:   "service-name",
			NumUnits:      1,
			ToMachineSpec: "42",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `cannot deploy "service-name" to machine 42: machine 42 not found`)

	_, err = s.State.Service("service-name")
	c.Assert(err, gc.ErrorMatches, `service "service-name" not found`)
}

func (s *serviceSuite) TestClientServiceDeployServiceOwner(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	results, err := s.serviceApi.ServicesDeploy(params.ServicesDeploy{
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
	err = s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl.String())
	c.Assert(force, gc.Equals, forceCharmUrl)
}

func (s *serviceSuite) TestClientServiceUpdateSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *serviceSuite) TestBlockDestroyServiceUpdate(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceUpdate")
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
	err := s.serviceApi.ServiceUpdate(args)
	s.AssertBlocked(c, err, "TestBlockChangeServiceUpdate")
}

func (s *serviceSuite) TestClientServiceUpdateForceSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, true)
}

func (s *serviceSuite) TestBlockServiceUpdateForced(c *gc.C) {
	curl := s.setupServiceUpdate(c)

	// block all changes. Force should ignore block :)
	s.BlockAllChanges(c, "TestBlockServiceUpdateForced")
	s.BlockDestroyEnvironment(c, "TestBlockServiceUpdateForced")
	s.BlockRemoveObject(c, "TestBlockServiceUpdateForced")

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      curl,
		ForceCharmUrl: true,
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl)
	c.Assert(force, jc.IsTrue)
}

func (s *serviceSuite) TestClientServiceUpdateSetCharmNotFound(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	args := params.ServiceUpdate{
		ServiceName: "wordpress",
		CharmUrl:    "cs:precise/wordpress-999999",
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Check(err, gc.ErrorMatches, `charm "cs:precise/wordpress-999999" not found`)
}

func (s *serviceSuite) TestClientServiceUpdateSetMinUnits(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set minimum units for the service.
	minUnits := 2
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, minUnits)
}

func (s *serviceSuite) TestClientServiceUpdateSetMinUnitsError(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set a negative minimum number of units for the service.
	minUnits := -1
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for service "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, 0)
}

func (s *serviceSuite) TestClientServiceUpdateSetSettingsStrings(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:     "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestClientServiceUpdateSetSettingsYAML(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "dummy:\n  title: y-title\n  username: y-user",
	}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestClientServiceUpdateSetConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		Constraints: &cons,
	}
	err = s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *serviceSuite) TestClientServiceUpdateAllParams(c *gc.C) {
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
	err = s.serviceApi.ServiceUpdate(args)
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

func (s *serviceSuite) TestClientServiceUpdateNoParams(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Calling ServiceUpdate with no parameters set is a no-op.
	args := params.ServiceUpdate{ServiceName: "wordpress"}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestClientServiceUpdateNoService(c *gc.C) {
	err := s.serviceApi.ServiceUpdate(params.ServiceUpdate{})
	c.Assert(err, gc.ErrorMatches, `"" is not a valid service name`)
}

func (s *serviceSuite) TestClientServiceUpdateInvalidService(c *gc.C) {
	args := params.ServiceUpdate{ServiceName: "no-such-service"}
	err := s.serviceApi.ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches, `service "no-such-service" not found`)
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
