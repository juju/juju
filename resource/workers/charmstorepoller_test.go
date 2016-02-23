// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/resource/workers"
	"github.com/juju/juju/worker"
	workertest "github.com/juju/juju/worker/testing"
)

type CharmStorePollerSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	deps   *stubCharmStorePollerDeps
	client *stubCharmStoreClient
}

var _ = gc.Suite(&CharmStorePollerSuite{})

func (s *CharmStorePollerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.deps = &stubCharmStorePollerDeps{Stub: s.stub}
	s.client = &stubCharmStoreClient{Stub: s.stub}
	s.deps.ReturnNewClient = s.client
}

func (s *CharmStorePollerSuite) TestIntegration(c *gc.C) {
	s.deps.ReturnListAllServices = []workers.Service{
		newStubService(c, s.stub, "svc-a"),
		newStubService(c, s.stub, "svc-b"),
	}
	s.client.ReturnListResources = [][]charmresource.Resource{{
		resourcetesting.NewCharmResource(c, "spam", "blahblahblah"),
	}, {
		resourcetesting.NewCharmResource(c, "eggs", "..."),
		resourcetesting.NewCharmResource(c, "ham", "lahdeedah"),
	}}
	done := make(chan struct{})
	poller := workers.NewCharmStorePoller(s.deps, s.deps.NewClient)
	poller.CharmStorePollerDeps = &doTracker{
		CharmStorePollerDeps: poller.CharmStorePollerDeps,
		done:                 done,
	}

	worker := poller.NewWorker()
	go func() {
		<-done
		worker.Kill()
	}()
	err := worker.Wait()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListAllServices",
		"CharmURL",
		"CharmURL",
		"NewClient",
		"ListResources",
		"Close",
		"ID",
		"SetCharmStoreResources",
		"ID",
		"SetCharmStoreResources",
	)
}

func (s *CharmStorePollerSuite) TestNewCharmStorePoller(c *gc.C) {
	poller := workers.NewCharmStorePoller(s.deps, s.deps.NewClient)

	s.stub.CheckNoCalls(c)
	c.Check(poller.Period, gc.Equals, 24*time.Hour)
}

func (s *CharmStorePollerSuite) TestNewWorker(c *gc.C) {
	expected := &workertest.StubWorker{Stub: s.stub}
	s.deps.ReturnNewPeriodicWorker = expected
	period := 11 * time.Second
	poller := workers.CharmStorePoller{
		CharmStorePollerDeps: s.deps,
		Period:               period,
	}

	worker := poller.NewWorker()

	s.stub.CheckCallNames(c, "NewPeriodicWorker")
	c.Check(worker, gc.Equals, expected)
}

func (s *CharmStorePollerSuite) TestDo(c *gc.C) {
	s.deps.ReturnListAllServices = []workers.Service{
		newStubService(c, s.stub, "svc-a"),
		newStubService(c, s.stub, "svc-b"),
	}
	s.deps.ReturnListCharmStoreResources = [][]charmresource.Resource{{
		resourcetesting.NewCharmResource(c, "spam", "blahblahblah"),
	}, {
		resourcetesting.NewCharmResource(c, "eggs", "..."),
		resourcetesting.NewCharmResource(c, "ham", "lahdeedah"),
	}}
	poller := workers.CharmStorePoller{
		CharmStorePollerDeps: s.deps,
	}
	stop := make(chan struct{})
	defer close(stop)

	err := poller.Do(stop)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListAllServices",
		"CharmURL",
		"CharmURL",
		"ListCharmStoreResources",
		"ID",
		"SetCharmStoreResources",
		"ID",
		"SetCharmStoreResources",
	)
}

type stubCharmStorePollerDeps struct {
	*testing.Stub

	ReturnNewClient               workers.CharmStoreClient
	ReturnListAllServices         []workers.Service
	ReturnNewPeriodicWorker       worker.Worker
	ReturnListCharmStoreResources [][]charmresource.Resource
}

func (s *stubCharmStorePollerDeps) NewClient() (workers.CharmStoreClient, error) {
	s.AddCall("NewClient")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnNewClient, nil
}

func (s *stubCharmStorePollerDeps) ListAllServices() ([]workers.Service, error) {
	s.AddCall("ListAllServices")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListAllServices, nil
}

func (s *stubCharmStorePollerDeps) SetCharmStoreResources(serviceID string, info []charmresource.Resource, lastPolled time.Time) error {
	s.AddCall("SetCharmStoreResources", serviceID, info, lastPolled)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubCharmStorePollerDeps) NewPeriodicWorker(call func(stop <-chan struct{}) error, period time.Duration) worker.Worker {
	s.AddCall("NewPeriodicWorker", call, period)
	s.NextErr() // Pop one off.

	return s.ReturnNewPeriodicWorker
}

func (s *stubCharmStorePollerDeps) ListCharmStoreResources(cURLs []*charm.URL) ([][]charmresource.Resource, error) {
	s.AddCall("ListCharmStoreResources", cURLs)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListCharmStoreResources, nil
}

type stubCharmStoreClient struct {
	*testing.Stub

	ReturnListResources [][]charmresource.Resource
}

func (s *stubCharmStoreClient) ListResources(cURLs []*charm.URL) ([][]charmresource.Resource, error) {
	s.AddCall("ListResources", cURLs)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubCharmStoreClient) Close() error {
	s.AddCall("Close")
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubService struct {
	*testing.Stub

	ReturnID       names.ServiceTag
	ReturnCharmURL *charm.URL
}

func newStubService(c *gc.C, stub *testing.Stub, name string) *stubService {
	cURL := &charm.URL{
		Schema:   "cs",
		Name:     name,
		Revision: 1,
	}
	return &stubService{
		Stub:           stub,
		ReturnID:       names.NewServiceTag(name),
		ReturnCharmURL: cURL,
	}
}

func (s *stubService) ID() names.ServiceTag {
	s.AddCall("ID")
	s.NextErr() // Pop one off.

	return s.ReturnID
}

func (s *stubService) CharmURL() *charm.URL {
	s.AddCall("CharmURL")
	s.NextErr() // Pop one off.

	return s.ReturnCharmURL
}

type doTracker struct {
	workers.CharmStorePollerDeps

	done chan struct{}
}

func (dt doTracker) NewPeriodicWorker(call func(stop <-chan struct{}) error, period time.Duration) worker.Worker {
	wrapper := func(stop <-chan struct{}) error {
		err := call(stop)
		close(dt.done)
		return err
	}
	return dt.CharmStorePollerDeps.NewPeriodicWorker(wrapper, period)
}
