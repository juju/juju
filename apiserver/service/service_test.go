// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"
	"io"
	"sync"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charmstore.v4/csclient"
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
	c.Assert(err, gc.ErrorMatches, `charm URL has invalid schema: ":nonsense-0"`)
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
	err := s.Srv.NewClient().Put("/"+curl.Path()+"/meta/perm/read", []string{"bob"})
	c.Assert(err, jc.ErrorIsNil)

	// Try to add a charm to the environment without authorization.
	s.DischargeUser = ""
	err = s.APIState.Client().AddCharm(curl)
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: cannot get discharge from ".*": third party refused discharge: cannot discharge: discharge denied`)

	tryAs := func(user string) error {
		client := csclient.New(csclient.Params{
			URL: s.Srv.URL(),
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
