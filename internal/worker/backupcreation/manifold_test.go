// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	dt "github.com/juju/worker/v5/dependency/testing"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) { tc.Run(t, &manifoldSuite{}) }

func manifoldConfig(c *tc.C) ManifoldConfig {
	worker := workerConfig(c)
	return ManifoldConfig{
		LeaseManagerName: "leases", DomainServicesName: "services", ObjectStoreName: "objects",
		ControllerUUID: worker.ControllerUUID, ControllerModelUUID: worker.ControllerModelUUID,
		ControllerNodeID:  worker.ControllerNodeID,
		ControllerTag:     names.NewControllerAgentTag("0"),
		LocalConfigReader: localConfigStub{values: LocalValues{DataDir: "/data", LogDir: "/logs"}},
		Clock:             worker.Clock, Logger: worker.Logger,
		NewWorker: NewWorker,
	}
}

func manifoldResources() map[string]any {
	return map[string]any{
		"leases":   leaseManagerStub{},
		"services": servicesGetterStub{},
		"objects":  objectGetterStub{},
	}
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Check(Manifold(manifoldConfig(c)).Inputs, tc.SameContents, []string{"leases", "services", "objects"})
}

func (s *manifoldSuite) TestStartAndOutputDeferServicesUntilCreation(c *tc.C) {
	cfg := manifoldConfig(c)
	failure := errors.New("services unavailable")
	var claimed, revoked bool
	resources := manifoldResources()
	resources["leases"] = leaseManagerStub{operations: leaseOperations{
		claim:  func(string, string, time.Duration) error { claimed = true; return nil },
		revoke: func(string, string) error { revoked = true; return nil },
	}}
	type contextKey struct{}
	resources["services"] = servicesGetterStub{get: func(ctx context.Context, model coremodel.UUID) (services.DomainServices, error) {
		c.Check(claimed, tc.IsTrue)
		c.Check(model.String(), tc.Equals, cfg.ControllerModelUUID)
		c.Check(ctx.Value(contextKey{}), tc.Equals, "request")
		return nil, failure
	}}
	manifold := Manifold(cfg)
	w, err := manifold.Start(c.Context(), dt.StubGetter(resources))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })
	c.Check(claimed, tc.IsFalse)
	var create corebackups.BackupCreator
	c.Assert(manifold.Output(w, &create), tc.ErrorIsNil)
	_, _, _, err = create.Create(context.WithValue(c.Context(), contextKey{}, "request"), "notes")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(revoked, tc.IsTrue)
	var wrong string
	c.Check(manifold.Output(w, &wrong), tc.ErrorMatches, "expected backup creation function, got .*string")
	c.Check(manifold.Output(nil, &create), tc.ErrorMatches, "expected backup creation worker, got .*nil.*")
}

func (s *manifoldSuite) TestObjectStoreFailure(c *tc.C) {
	cfg := manifoldConfig(c)
	failure := errors.New("object store unavailable")
	resources := manifoldResources()
	resources["services"] = servicesGetterStub{get: func(context.Context, coremodel.UUID) (services.DomainServices, error) {
		return domainServicesStub{}, nil
	}}
	resources["objects"] = objectGetterStub{get: func(ctx context.Context, namespace string) (objectstore.ObjectStore, error) {
		c.Check(ctx.Err(), tc.ErrorIsNil)
		c.Check(namespace, tc.Equals, database.ControllerNS)
		return nil, failure
	}}
	manifold := Manifold(cfg)
	w, err := manifold.Start(c.Context(), dt.StubGetter(resources))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })
	var create corebackups.BackupCreator
	c.Assert(manifold.Output(w, &create), tc.ErrorIsNil)
	_, _, _, err = create.Create(c.Context(), "notes")
	c.Check(err, tc.ErrorIs, failure)
}

func (s *manifoldSuite) TestMissingDependencies(c *tc.C) {
	for _, name := range []string{"leases", "services", "objects"} {
		resources := manifoldResources()
		delete(resources, name)
		_, err := Manifold(manifoldConfig(c)).Start(c.Context(), dt.StubGetter(resources))
		c.Check(err, tc.NotNil)
	}
}

func (s *manifoldSuite) TestInvalidConfiguration(c *tc.C) {
	tests := []struct {
		change  func(*ManifoldConfig)
		message string
	}{
		{func(c *ManifoldConfig) { c.LeaseManagerName = "" }, "empty LeaseManagerName not valid"},
		{func(c *ManifoldConfig) { c.DomainServicesName = "" }, "empty DomainServicesName not valid"},
		{func(c *ManifoldConfig) { c.ObjectStoreName = "" }, "empty ObjectStoreName not valid"},
		{func(c *ManifoldConfig) { c.ControllerTag = nil }, "nil ControllerTag not valid"},
		{func(c *ManifoldConfig) { c.ControllerNodeID = "" }, "empty ControllerNodeID not valid"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = nil }, "nil LocalConfigReader not valid"},
		{func(c *ManifoldConfig) { c.Clock = nil }, "nil Clock not valid"},
		{func(c *ManifoldConfig) { c.Logger = nil }, "nil Logger not valid"},
		{func(c *ManifoldConfig) { c.NewWorker = nil }, "nil NewWorker not valid"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = localConfigStub{} }, "empty backup paths not valid"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = localConfigStub{err: errors.New("paths failed")} }, "reading backup paths: paths failed"},
	}
	for _, test := range tests {
		cfg := manifoldConfig(c)
		test.change(&cfg)
		_, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(manifoldResources()))
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *manifoldSuite) TestWorkerFailure(c *tc.C) {
	cfg := manifoldConfig(c)
	failure := errors.New("cannot start worker")
	cfg.NewWorker = func(Config) (worker.Worker, error) { return nil, failure }
	_, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(manifoldResources()))
	c.Check(err, tc.ErrorIs, failure)
}

type localConfigStub struct {
	values LocalValues
	err    error
}

func (s localConfigStub) BackupLocalValues() (LocalValues, error) { return s.values, s.err }

type domainServicesStub struct{ services.DomainServices }
type servicesGetterStub struct {
	get func(context.Context, coremodel.UUID) (services.DomainServices, error)
}

func (s servicesGetterStub) ServicesForModel(ctx context.Context, model coremodel.UUID) (services.DomainServices, error) {
	return s.get(ctx, model)
}

type objectGetterStub struct {
	get func(context.Context, string) (objectstore.ObjectStore, error)
}

func (s objectGetterStub) GetObjectStore(ctx context.Context, namespace string) (objectstore.ObjectStore, error) {
	return s.get(ctx, namespace)
}
