// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	accessservice "github.com/juju/juju/domain/access/service"
	domaincontroller "github.com/juju/juju/domain/controller"
	controllerservice "github.com/juju/juju/domain/controller/service"
	loggingservice "github.com/juju/juju/domain/logging/service"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/socketlistener"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.ObjectStoreName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewSocketListener = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.SocketName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetControllerDomainServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetControllerObjectStoreService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetObjectStoreServicesGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetReadRepairObjectStoreGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.PrometheusRegisterer = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewMetricsCollector = nil
	c.Check(cfg.Validate(), tc.ErrorIs, jujuerrors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(Manifold(cfg).Inputs, tc.DeepEquals, []string{
		cfg.DomainServicesName,
		cfg.ObjectStoreName,
		cfg.ObjectStoreServicesName,
	})
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	controllerModelUUID := model.UUID("01234567-89ab-cdef-0123-456789abcdef")

	domainServices := manifoldStubControllerDomainServices{
		controllerService: controllerservice.NewService(manifoldStubControllerState{
			modelUUID: controllerModelUUID,
		}),
	}
	controllerObjectStore := &manifoldStubControllerObjectStoreService{}
	objectStoreServicesGetter := manifoldStubObjectStoreServicesGetter{
		metadataByModel: map[model.UUID]MetadataService{},
	}
	readRepairGetter := &manifoldStubReadRepairGetter{}

	var gotWorkerConfig Config
	cfg := s.getConfig(c)
	cfg.GetControllerDomainServices = func(
		dependency.Getter, string,
	) (services.ControllerDomainServices, error) {
		return domainServices, nil
	}
	cfg.GetControllerObjectStoreService = func(
		dependency.Getter, string,
	) (ControllerObjectStoreService, error) {
		return controllerObjectStore, nil
	}
	cfg.GetObjectStoreServicesGetter = func(
		dependency.Getter, string,
	) (ObjectStoreServicesGetter, error) {
		return objectStoreServicesGetter, nil
	}
	cfg.GetReadRepairObjectStoreGetter = func(
		dependency.Getter, string,
	) (ReadRepairObjectStoreGetter, error) {
		return readRepairGetter, nil
	}
	cfg.NewWorker = func(cfg Config) (worker.Worker, error) {
		gotWorkerConfig = cfg
		return workertest.NewErrorWorker(nil), nil
	}

	w, err := Manifold(cfg).Start(c.Context(), dependencytesting.StubGetter(map[string]any{}))
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	c.Check(gotWorkerConfig.ControllerModelUUID, tc.Equals, controllerModelUUID)
	c.Check(gotWorkerConfig.ObjectStoreService, tc.Equals, controllerObjectStore)
	c.Check(gotWorkerConfig.ReadRepairObjectStoreGetter, tc.Equals, readRepairGetter)
	c.Check(gotWorkerConfig.DrainPreflightValidator, tc.NotNil)
}

func (s *manifoldSuite) TestStartReadRepairGetterError(c *tc.C) {
	domainServices := manifoldStubControllerDomainServices{}
	controllerObjectStore := &manifoldStubControllerObjectStoreService{}
	objectStoreServicesGetter := manifoldStubObjectStoreServicesGetter{
		metadataByModel: map[model.UUID]MetadataService{},
	}

	newWorkerCalled := false
	cfg := s.getConfig(c)
	cfg.GetControllerDomainServices = func(
		dependency.Getter, string,
	) (services.ControllerDomainServices, error) {
		return domainServices, nil
	}
	cfg.GetControllerObjectStoreService = func(
		dependency.Getter, string,
	) (ControllerObjectStoreService, error) {
		return controllerObjectStore, nil
	}
	cfg.GetObjectStoreServicesGetter = func(
		dependency.Getter, string,
	) (ObjectStoreServicesGetter, error) {
		return objectStoreServicesGetter, nil
	}
	cfg.GetReadRepairObjectStoreGetter = func(
		dependency.Getter, string,
	) (ReadRepairObjectStoreGetter, error) {
		return nil, jujuerrors.New("boom")
	}
	cfg.NewWorker = func(Config) (worker.Worker, error) {
		newWorkerCalled = true
		return workertest.NewErrorWorker(nil), nil
	}

	_, err := Manifold(cfg).Start(c.Context(), dependencytesting.StubGetter(map[string]any{}))
	c.Assert(err, tc.ErrorMatches, ".*boom.*")
	c.Check(newWorkerCalled, tc.IsFalse)
}

func (s *manifoldSuite) TestGetControllerObjectStoreService(c *tc.C) {
	agentObjectStore := &objectstoreservice.WatchableDrainingService{}
	storeServices := manifoldStubControllerObjectStoreServices{
		agentObjectStore: agentObjectStore,
	}
	getter := dependencytesting.StubGetter(map[string]any{
		"object-store-services": storeServices,
	})

	svc, err := GetControllerObjectStoreService(getter, "object-store-services")
	c.Assert(err, tc.ErrorIsNil)

	got, ok := svc.(*objectstoreservice.WatchableDrainingService)
	c.Assert(ok, tc.IsTrue)
	c.Check(got, tc.Equals, agentObjectStore)
}

func (s *manifoldSuite) TestGetObjectStoreServicesGetter(c *tc.C) {
	modelUUID := model.UUID("11111111-2222-3333-4444-555555555555")
	objectStore := &objectstoreservice.WatchableService{}
	storeServices := manifoldStubObjectStoreServices{
		objectStore: objectStore,
	}
	servicesGetter := &manifoldStubServicesObjectStoreServicesGetter{
		servicesForModel: storeServices,
	}
	getter := dependencytesting.StubGetter(map[string]any{
		"object-store-services": servicesGetter,
	})

	metadataGetter, err := GetObjectStoreServicesGetter(getter, "object-store-services")
	c.Assert(err, tc.ErrorIsNil)

	got := metadataGetter.ObjectStoreForModel(modelUUID)
	c.Check(got, tc.Equals, objectStore)
	c.Check(servicesGetter.modelUUID, tc.Equals, modelUUID)
}

func (s *manifoldSuite) TestGetReadRepairObjectStoreGetter(c *tc.C) {
	store := &manifoldStubCoreObjectStore{}
	coreGetter := &manifoldStubCoreObjectStoreGetter{
		store: store,
	}
	getter := dependencytesting.StubGetter(map[string]any{
		"object-store": coreGetter,
	})

	readRepairGetter, err := GetReadRepairObjectStoreGetter(getter, "object-store")
	c.Assert(err, tc.ErrorIsNil)

	readStore, err := readRepairGetter.GetObjectStore(c.Context(), "controller")
	c.Assert(err, tc.ErrorIsNil)

	reader, _, err := readStore.Get(c.Context(), "tools/juju")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader.Close(), tc.ErrorIsNil)

	c.Check(coreGetter.namespace, tc.Equals, "controller")
	c.Check(store.path, tc.Equals, "tools/juju")
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName:      "domain-services",
		ObjectStoreName:         "object-store",
		ObjectStoreServicesName: "object-store-services",
		Logger:                  loggertesting.WrapCheckLog(c),
		NewWorker: func(Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		NewSocketListener: func(socketlistener.Config) (SocketListener, error) {
			return nil, nil
		},
		SocketName: filepath.Join(c.MkDir(), "control.socket"),
		GetControllerDomainServices: func(
			dependency.Getter, string,
		) (services.ControllerDomainServices, error) {
			return nil, nil
		},
		GetControllerObjectStoreService: func(
			dependency.Getter, string,
		) (ControllerObjectStoreService, error) {
			return &manifoldStubControllerObjectStoreService{}, nil
		},
		GetObjectStoreServicesGetter: func(
			dependency.Getter, string,
		) (ObjectStoreServicesGetter, error) {
			return manifoldStubObjectStoreServicesGetter{
				metadataByModel: map[model.UUID]MetadataService{},
			}, nil
		},
		GetReadRepairObjectStoreGetter: func(
			dependency.Getter, string,
		) (ReadRepairObjectStoreGetter, error) {
			return &manifoldStubReadRepairGetter{}, nil
		},
		PrometheusRegisterer: prometheus.NewRegistry(),
		NewMetricsCollector:  NewMetricsCollector,
	}
}

type manifoldStubControllerState struct {
	modelUUID model.UUID
}

func (s manifoldStubControllerState) GetControllerModelUUID(context.Context) (model.UUID, error) {
	return s.modelUUID, nil
}

func (manifoldStubControllerState) GetControllerAgentInfo(context.Context) (controller.ControllerAgentInfo, error) {
	return controller.ControllerAgentInfo{}, nil
}

func (manifoldStubControllerState) GetModelNamespaces(context.Context) ([]string, error) {
	return nil, nil
}

func (manifoldStubControllerState) GetCACert(context.Context) (string, error) {
	return "", nil
}

func (manifoldStubControllerState) GetControllerInfo(context.Context) (domaincontroller.ControllerInfo, error) {
	return domaincontroller.ControllerInfo{}, nil
}

type manifoldStubControllerDomainServices struct {
	services.ControllerDomainServices
	controllerService *controllerservice.Service
}

func (manifoldStubControllerDomainServices) Access() *accessservice.Service {
	return nil
}

func (s manifoldStubControllerDomainServices) Controller() *controllerservice.Service {
	return s.controllerService
}

func (manifoldStubControllerDomainServices) Logging() *loggingservice.WatchableService {
	return nil
}

func (manifoldStubControllerDomainServices) Tracing() *tracingservice.WatchableService {
	return nil
}

type manifoldStubControllerObjectStoreService struct {
	metadata []coreobjectstore.Metadata
	err      error
}

func (*manifoldStubControllerObjectStoreService) GetActiveObjectStoreBackend(
	context.Context,
) (objectstoreservice.BackendInfo, error) {
	return objectstoreservice.BackendInfo{
		Type: coreobjectstore.FileBackend,
	}, nil
}

func (s *manifoldStubControllerObjectStoreService) TransitionBackendToS3(
	context.Context, domainobjectstore.S3Credentials,
) error {
	return nil
}

func (s *manifoldStubControllerObjectStoreService) ListMetadata(
	context.Context,
) ([]coreobjectstore.Metadata, error) {
	return s.metadata, s.err
}

type manifoldStubControllerObjectStoreServices struct {
	services.ControllerObjectStoreServices
	agentObjectStore *objectstoreservice.WatchableDrainingService
}

func (s manifoldStubControllerObjectStoreServices) AgentObjectStore() *objectstoreservice.WatchableDrainingService {
	return s.agentObjectStore
}

type manifoldStubObjectStoreServicesGetter struct {
	metadataByModel map[model.UUID]MetadataService
}

func (s manifoldStubObjectStoreServicesGetter) ObjectStoreForModel(modelUUID model.UUID) MetadataService {
	return s.metadataByModel[modelUUID]
}

type manifoldStubReadRepairGetter struct{}

func (*manifoldStubReadRepairGetter) GetObjectStore(
	context.Context, string,
) (ReadRepairObjectStore, error) {
	return nil, nil
}

type manifoldStubServicesObjectStoreServicesGetter struct {
	services.ObjectStoreServicesGetter
	servicesForModel services.ObjectStoreServices
	modelUUID        model.UUID
}

func (s *manifoldStubServicesObjectStoreServicesGetter) ServicesForModel(modelUUID model.UUID) services.ObjectStoreServices {
	s.modelUUID = modelUUID
	return s.servicesForModel
}

type manifoldStubObjectStoreServices struct {
	services.ObjectStoreServices
	objectStore *objectstoreservice.WatchableService
}

func (s manifoldStubObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return s.objectStore
}

type manifoldStubCoreObjectStoreGetter struct {
	store     coreobjectstore.ObjectStore
	namespace string
}

func (g *manifoldStubCoreObjectStoreGetter) GetObjectStore(
	_ context.Context, namespace string,
) (coreobjectstore.ObjectStore, error) {
	g.namespace = namespace
	return g.store, nil
}

type manifoldStubCoreObjectStore struct {
	coreobjectstore.ObjectStore
	path string
}

func (s *manifoldStubCoreObjectStore) Get(
	_ context.Context, path string,
) (io.ReadCloser, coreobjectstore.Digest, error) {
	s.path = path
	return io.NopCloser(strings.NewReader("data")), coreobjectstore.Digest{}, nil
}
