// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	dt "github.com/juju/worker/v5/dependency/testing"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) { tc.Run(t, &manifoldSuite{}) }

func manifoldConfig(c *tc.C) ManifoldConfig {
	worker := workerConfig(c)
	return ManifoldConfig{
		LeaseManagerName: "leases", DomainServicesName: "services", ObjectStoreName: "objects",
		ControllerUUID: worker.ControllerUUID, ControllerModelUUID: worker.ControllerModelUUID,
		ControllerNodeID:  worker.ControllerNodeID,
		LocalConfigReader: localConfigStub{values: LocalValues{DataDir: "/data", LogDir: "/logs"}},
		Clock:             worker.Clock, Logger: worker.Logger,
		NewWorker:       NewWorker,
		NewRecordSource: MachineRecordSource,
	}
}

// k8sVariant returns the configuration the Kubernetes agent wires: the
// Kubernetes source recorder, which reads the provider tracker.
func k8sVariant(cfg ManifoldConfig) ManifoldConfig {
	cfg.ProviderTrackerName = "providers"
	cfg.NewRecordSource = K8sRecordSource
	return cfg
}

func manifoldResources() map[string]any {
	return map[string]any{
		"leases":    leaseManagerStub{},
		"services":  servicesGetterStub{},
		"objects":   objectGetterStub{},
		"providers": providerFactoryStub{},
	}
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	// The machine wiring reads no provider, so the manifold inputs the
	// lease manager, the domain services and the object store only.
	c.Check(Manifold(manifoldConfig(c)).Inputs, tc.SameContents,
		[]string{"leases", "services", "objects"})

	// The Kubernetes wiring reads the provider tracker.
	c.Check(Manifold(k8sVariant(manifoldConfig(c))).Inputs, tc.SameContents,
		[]string{"leases", "services", "objects", "providers"})
}

func (s *manifoldSuite) TestStartAndOutputDeferServicesUntilCreation(c *tc.C) {
	cfg := manifoldConfig(c)
	manifold := Manifold(cfg)
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
	manifold := Manifold(manifoldConfig(c))
	w, err := manifold.Start(c.Context(), dt.StubGetter(resources))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })
	var create corebackups.BackupCreator
	c.Assert(manifold.Output(w, &create), tc.ErrorIsNil)
	_, _, _, err = create.Create(c.Context(), "notes")
	c.Check(err, tc.ErrorIs, failure)
}

func (s *manifoldSuite) TestMissingDependencies(c *tc.C) {
	// The machine recorder reads no provider, so the lease manager, the
	// domain services and the object store are the resources every
	// substrate needs.
	for _, name := range []string{"leases", "services", "objects"} {
		resources := manifoldResources()
		delete(resources, name)
		_, err := Manifold(manifoldConfig(c)).Start(c.Context(), dt.StubGetter(resources))
		c.Check(err, tc.NotNil)
	}

	// The Kubernetes recorder resolves the provider factory from the
	// tracker resource.
	resources := manifoldResources()
	delete(resources, "providers")
	_, err := Manifold(k8sVariant(manifoldConfig(c))).Start(c.Context(), dt.StubGetter(resources))
	c.Check(err, tc.NotNil)
}

func (s *manifoldSuite) TestInvalidConfiguration(c *tc.C) {
	tests := []struct {
		change  func(*ManifoldConfig)
		message string
	}{
		{func(c *ManifoldConfig) { c.LeaseManagerName = "" }, "empty LeaseManagerName not valid"},
		{func(c *ManifoldConfig) { c.DomainServicesName = "" }, "empty DomainServicesName not valid"},
		{func(c *ManifoldConfig) { c.ObjectStoreName = "" }, "empty ObjectStoreName not valid"},
		{func(c *ManifoldConfig) { c.ControllerNodeID = "" }, "empty ControllerNodeID not valid"},
		{func(c *ManifoldConfig) { c.ControllerUUID = "bad" }, "parsing controller UUID: .*"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = nil }, "nil LocalConfigReader not valid"},
		{func(c *ManifoldConfig) { c.Clock = nil }, "nil Clock not valid"},
		{func(c *ManifoldConfig) { c.Logger = nil }, "nil Logger not valid"},
		{func(c *ManifoldConfig) { c.NewWorker = nil }, "nil NewWorker not valid"},
		{func(c *ManifoldConfig) { c.NewRecordSource = nil }, "nil NewRecordSource not valid"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = localConfigStub{} }, "empty backup paths not valid"},
		{func(c *ManifoldConfig) { c.LocalConfigReader = localConfigStub{err: errors.New("paths failed")} }, "reading backup paths: paths failed"},
	}
	for _, test := range tests {
		cfg := manifoldConfig(c)
		test.change(&cfg)
		_, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(manifoldResources()))
		c.Check(err, tc.ErrorMatches, test.message)
	}

	// Only the Kubernetes recorder reads the provider tracker, so only
	// it requires the name.
	k8s := k8sVariant(manifoldConfig(c))
	k8s.ProviderTrackerName = ""
	_, err := Manifold(k8s).Start(c.Context(), dt.StubGetter(manifoldResources()))
	c.Check(err, tc.ErrorMatches, "empty ProviderTrackerName not valid")
}

func (s *manifoldSuite) TestWorkerFailure(c *tc.C) {
	failure := errors.New("cannot start worker")
	cfg := manifoldConfig(c)
	cfg.NewWorker = func(Config) (worker.Worker, error) { return nil, failure }
	_, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(manifoldResources()))
	c.Check(err, tc.ErrorIs, failure)
}

// TestManifoldWiresRecordSource verifies that the manifold resolves the
// configuration and resources at start, hands them to the wired recorder
// constructor, and builds the archive generator with the recorder it
// returns.
func (s *manifoldSuite) TestManifoldWiresRecordSource(c *tc.C) {
	cfg := manifoldConfig(c)
	failure := errors.New("services unavailable")
	var gotArgs RecordSourceArgs
	cfg.NewRecordSource = func(args RecordSourceArgs) (RecordSourceFunc, error) {
		gotArgs = args
		return MachineRecordSource(args)
	}
	var startConfig Config
	cfg.NewWorker = func(config Config) (worker.Worker, error) {
		startConfig = config
		return NewWorker(config)
	}
	resources := manifoldResources()
	resources["services"] = servicesGetterStub{get: func(context.Context, coremodel.UUID) (services.DomainServices, error) {
		return nil, failure
	}}
	w, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(resources))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })

	// The constructor received the configuration and the resources
	// resolved at start.
	c.Check(gotArgs.Config.ControllerUUID, tc.Equals, cfg.ControllerUUID)
	c.Check(gotArgs.Config.ControllerModelUUID, tc.Equals, cfg.ControllerModelUUID)
	c.Check(gotArgs.Config.ControllerNodeID, tc.Equals, cfg.ControllerNodeID)
	c.Check(gotArgs.Config.ProviderTrackerName, tc.Equals, cfg.ProviderTrackerName)
	c.Check(gotArgs.DomainServices, tc.NotNil)
	c.Check(gotArgs.ModelServicesFor, tc.NotNil)
	controllerUUID, err := uuid.UUIDFromString(cfg.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotArgs.ControllerUUID, tc.Equals, controllerUUID)

	// The archive generator carries the recorder the constructor
	// returned.
	generator, ok := startConfig.CreateArchive.(archiveGenerator)
	c.Assert(ok, tc.IsTrue)
	c.Check(generator.recordSource, tc.NotNil)
	var meta corebackups.Metadata
	err = generator.recordSource(c.Context(), &meta)
	c.Check(err, tc.ErrorMatches, "recording controller base for recovery: services unavailable")
}

// TestMachineRecordSource verifies that the machine recorder constructor
// returns a recorder resolving the controller machine's base through the
// controller model's services.
func (s *manifoldSuite) TestMachineRecordSource(c *tc.C) {
	failure := errors.New("services unavailable")
	args := RecordSourceArgs{
		Config: manifoldConfig(c),
		ModelServicesFor: ModelServicesForFunc(func(context.Context, coremodel.UUID) (ModelExportDomainServices, error) {
			return nil, failure
		}),
	}
	recordSource, err := MachineRecordSource(args)
	c.Assert(err, tc.ErrorIsNil)

	var meta corebackups.Metadata
	err = recordSource(c.Context(), &meta)
	c.Check(err, tc.ErrorMatches, "recording controller base for recovery: services unavailable")
}

// TestK8sRecordSource verifies that the Kubernetes recorder constructor
// resolves the provider factory from the configured provider tracker and
// returns a recorder reading the controller's provisioning facts through
// it, deferring the controller model's services to each call.
func (s *manifoldSuite) TestK8sRecordSource(c *tc.C) {
	cfg := k8sVariant(manifoldConfig(c))
	failure := errors.New("services unavailable")
	calls := 0
	controllerUUID, err := uuid.UUIDFromString(cfg.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	args := RecordSourceArgs{
		Config: cfg,
		Getter: dt.StubGetter(map[string]any{
			cfg.ProviderTrackerName: &provisioningProviderFactoryStub{},
		}),
		DomainServices: servicesGetterStub{get: func(context.Context, coremodel.UUID) (services.DomainServices, error) {
			calls++
			return nil, failure
		}},
		ControllerUUID: controllerUUID,
	}
	recordSource, err := K8sRecordSource(args)
	c.Assert(err, tc.ErrorIsNil)

	// The recorder records through the Kubernetes provisioning read,
	// which resolves the controller model's services on each call.
	var meta corebackups.Metadata
	err = recordSource(c.Context(), &meta)
	c.Check(err, tc.ErrorMatches, "recording Kubernetes provisioning: services unavailable")
	c.Check(calls, tc.Equals, 1)
	err = recordSource(c.Context(), &meta)
	c.Check(err, tc.ErrorMatches, "recording Kubernetes provisioning: services unavailable")
	c.Check(calls, tc.Equals, 2)
}

// localConfigStub stands in for the agent's local configuration reader.
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

type providerFactoryStub struct {
	providertracker.ProviderFactory
}

type provisioningProviderFactoryStub struct {
	providertracker.ProviderFactory
	provider providertracker.Provider
	err      error
	got      providertracker.EphemeralProviderConfig
	called   bool
}

func (f *provisioningProviderFactoryStub) EphemeralProviderFromConfig(
	_ context.Context, cfg providertracker.EphemeralProviderConfig,
) (providertracker.Provider, error) {
	f.called = true
	f.got = cfg
	return f.provider, f.err
}
