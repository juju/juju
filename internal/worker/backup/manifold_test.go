// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

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
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/cloudspec"
	environsconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) { tc.Run(t, &manifoldSuite{}) }

func manifoldConfig(c *tc.C) ManifoldConfig {
	worker := workerConfig(c)
	return ManifoldConfig{
		LeaseManagerName: "leases", DomainServicesName: "services", ObjectStoreName: "objects", ProviderTrackerName: "providers",
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
		"leases":    leaseManagerStub{},
		"services":  servicesGetterStub{},
		"objects":   objectGetterStub{},
		"providers": providerFactoryStub{},
	}
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Check(Manifold(manifoldConfig(c)).Inputs, tc.SameContents, []string{"leases", "services", "objects", "providers"})
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
	for _, name := range []string{"leases", "services", "objects", "providers"} {
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
		{func(c *ManifoldConfig) { c.ProviderTrackerName = "" }, "empty ProviderTrackerName not valid"},
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

// TestReadControllerProvisioning verifies that the generator resolves the
// controller model's cloud spec and config, creates an ephemeral CAAS
// provider, and reports each failure with its cause.
func (s *manifoldSuite) TestReadControllerProvisioning(c *tc.C) {
	worker := workerConfig(c)
	controllerUUID, err := uuid.UUIDFromString(worker.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	spec := cloudspec.CloudSpec{Type: "kubernetes", Name: "test"}
	cfg := &environsconfig.Config{}
	provisioning := &corebackups.KubernetesProvisioning{
		AgentImage:   "example.com/agent:latest",
		CharmImage:   "example.com/charm:latest",
		Architecture: "arm64",
	}

	tests := []struct {
		name             string
		cloudSpec        cloudSpecProviderStub
		modelConfig      modelConfigProviderStub
		provider         providertracker.Provider
		factoryErr       error
		wantErr          string
		wantNotSupported bool
		wantFactory      bool
		wantResult       *corebackups.KubernetesProvisioning
	}{
		{
			name:      "cloud spec failure",
			cloudSpec: cloudSpecProviderStub{err: errors.New("cloud spec failed")},
			wantErr:   "cloud spec failed",
		},
		{
			name:        "model config failure",
			cloudSpec:   cloudSpecProviderStub{spec: spec},
			modelConfig: modelConfigProviderStub{err: errors.New("model config failed")},
			wantErr:     "model config failed",
		},
		{
			name:        "provider creation failure",
			cloudSpec:   cloudSpecProviderStub{spec: spec},
			modelConfig: modelConfigProviderStub{cfg: cfg},
			factoryErr:  errors.New("provider creation failed"),
			wantErr:     "provider creation failed",
			wantFactory: true,
		},
		{
			name:             "provider cannot record provisioning",
			cloudSpec:        cloudSpecProviderStub{spec: spec},
			modelConfig:      modelConfigProviderStub{cfg: cfg},
			provider:         &unsupportedProviderStub{},
			wantErr:          "provider cannot read controller provisioning",
			wantNotSupported: true,
			wantFactory:      true,
		},
		{
			name:        "provisioning failure",
			cloudSpec:   cloudSpecProviderStub{spec: spec},
			modelConfig: modelConfigProviderStub{cfg: cfg},
			provider:    &provisioningProviderStub{err: errors.New("provisioning failed")},
			wantErr:     "provisioning failed",
			wantFactory: true,
		},
		{
			name:        "success",
			cloudSpec:   cloudSpecProviderStub{spec: spec},
			modelConfig: modelConfigProviderStub{cfg: cfg},
			provider:    &provisioningProviderStub{provisioning: provisioning},
			wantFactory: true,
			wantResult:  provisioning,
		},
	}
	for _, test := range tests {
		c.Logf("case: %s", test.name)
		factory := &provisioningProviderFactoryStub{
			provider: test.provider,
			err:      test.factoryErr,
		}
		generator := archiveGenerator{
			config: ManifoldConfig{
				ControllerUUID:   worker.ControllerUUID,
				ControllerNodeID: "7",
			},
			providerFactory: factory,
		}

		result, err := generator.readControllerProvisioning(c.Context(), test.cloudSpec, test.modelConfig)
		if test.wantErr != "" {
			c.Check(err, tc.ErrorMatches, test.wantErr)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(result, tc.Equals, test.wantResult)
		}
		if test.wantNotSupported {
			c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
		}
		if test.wantFactory {
			c.Check(factory.called, tc.IsTrue)
			c.Check(factory.got.ModelType, tc.Equals, coremodel.CAAS)
			c.Check(factory.got.ModelConfig, tc.Equals, cfg)
			c.Check(factory.got.CloudSpec, tc.DeepEquals, spec)
			c.Check(factory.got.ControllerUUID, tc.Equals, controllerUUID)
		} else {
			c.Check(factory.called, tc.IsFalse)
		}
		if provider, ok := test.provider.(*provisioningProviderStub); ok {
			c.Check(provider.called, tc.IsTrue)
			c.Check(provider.controllerID, tc.Equals, "7")
		}
	}
}

// TestStartWiresProviderFactoryIntoGenerator verifies that the provider
// factory resolved by the dependency engine reaches the archive generator
// that the manifold hands to the worker.
func (s *manifoldSuite) TestStartWiresProviderFactoryIntoGenerator(c *tc.C) {
	cfg := manifoldConfig(c)
	factory := &provisioningProviderFactoryStub{}
	resources := manifoldResources()
	resources["providers"] = factory

	var startConfig Config
	cfg.NewWorker = func(config Config) (worker.Worker, error) {
		startConfig = config
		return NewWorker(config)
	}
	w, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(resources))
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })

	generator, ok := startConfig.CreateArchive.(archiveGenerator)
	c.Assert(ok, tc.IsTrue)
	c.Check(generator.providerFactory, tc.Equals, factory)
}

// TestProvisioningReader verifies that the read handed to the creator
// resolves the provider factory's ephemeral provider and records the
// provisioning facts it reports.
func (s *manifoldSuite) TestProvisioningReader(c *tc.C) {
	worker := workerConfig(c)
	controllerUUID, err := uuid.UUIDFromString(worker.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	provisioning := &corebackups.KubernetesProvisioning{
		AgentImage:   "example.com/agent:latest",
		CharmImage:   "example.com/charm:latest",
		Architecture: "arm64",
	}
	factory := &provisioningProviderFactoryStub{
		provider: &provisioningProviderStub{provisioning: provisioning},
	}
	generator := archiveGenerator{
		config: ManifoldConfig{
			ControllerUUID:   worker.ControllerUUID,
			ControllerNodeID: "7",
		},
		providerFactory: factory,
	}
	spec := cloudspec.CloudSpec{Type: "kubernetes", Name: "test"}
	cfg := &environsconfig.Config{}

	read := generator.provisioningReader(
		cloudSpecProviderStub{spec: spec}, modelConfigProviderStub{cfg: cfg})
	result, err := read(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result, tc.DeepEquals, provisioning)
	c.Check(factory.called, tc.IsTrue)
	c.Check(factory.got.ModelType, tc.Equals, coremodel.CAAS)
	c.Check(factory.got.ModelConfig, tc.Equals, cfg)
	c.Check(factory.got.CloudSpec, tc.DeepEquals, spec)
	c.Check(factory.got.ControllerUUID, tc.Equals, controllerUUID)
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

type providerFactoryStub struct {
	providertracker.ProviderFactory
}

type cloudSpecProviderStub struct {
	spec cloudspec.CloudSpec
	err  error
}

func (s cloudSpecProviderStub) GetCloudSpec(context.Context) (cloudspec.CloudSpec, error) {
	return s.spec, s.err
}

type modelConfigProviderStub struct {
	cfg *environsconfig.Config
	err error
}

func (s modelConfigProviderStub) ModelConfig(context.Context) (*environsconfig.Config, error) {
	return s.cfg, s.err
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

type provisioningProviderStub struct {
	providertracker.Provider
	provisioning *corebackups.KubernetesProvisioning
	err          error
	controllerID string
	called       bool
}

func (p *provisioningProviderStub) ReadControllerProvisioning(
	_ context.Context, controllerID string,
) (*corebackups.KubernetesProvisioning, error) {
	p.called = true
	p.controllerID = controllerID
	return p.provisioning, p.err
}

// unsupportedProviderStub satisfies providertracker.Provider without
// implementing ControllerProvisioningReader.
type unsupportedProviderStub struct {
	providertracker.Provider
}
