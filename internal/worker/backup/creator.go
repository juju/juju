// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"
	"io"
	"os"
	"path"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	coreversion "github.com/juju/juju/core/version"
	domainexport "github.com/juju/juju/domain/export"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/recovery"
)

// Creator creates backup archives on demand. It is used by the
// controller's backups HTTP endpoint, which streams the archive to the
// client in the same request.
//
// The substrate-specific source facts are recorded through recordSource,
// which the manifold injects for the running controller's substrate.
type Creator struct {
	controllerUUID      string
	controllerModelUUID coremodel.UUID
	controllerID        string
	dataDir             string
	logDir              string

	controllerExport ControllerExportService
	modelServicesFor ModelServicesForFunc
	modelConfig      ModelConfigService
	controller       ControllerModelLister
	controllerNodes  ControllerNodeLister

	controllerObjectStore ReadObjectStore
	objectStoreForModel   ObjectStoreForModelFunc

	// recordSource records the substrate-specific source facts in the
	// archive metadata. The manifold injects the recorder for the
	// running controller's substrate; the creator holds no other
	// substrate-specific state.
	recordSource RecordSourceFunc

	clock  clock.Clock
	logger corelogger.Logger
}

// CreatorConfig supplies the dependencies needed to create backup
// archives. The substrate manifolds inject the source recorder for the
// running controller's substrate through RecordSource; the remaining
// dependencies are shared by both substrates.
type CreatorConfig struct {
	ControllerUUID        string
	ControllerModelUUID   coremodel.UUID
	ControllerID          string
	DataDir               string
	LogDir                string
	ControllerExport      ControllerExportService
	ModelServicesFor      ModelServicesForFunc
	ModelConfig           ModelConfigService
	Controller            ControllerModelLister
	ControllerNodes       ControllerNodeLister
	ControllerObjectStore ReadObjectStore
	ObjectStoreForModel   ObjectStoreForModelFunc
	// RecordSource records the substrate-specific source facts in the
	// archive metadata: the controller machine's base on machine
	// controllers, the controller's provisioning facts on Kubernetes
	// controllers.
	RecordSource RecordSourceFunc
	Clock        clock.Clock
	Logger       corelogger.Logger
}

// Validate checks that the substrate-independent archive creation
// dependencies are complete.
func (c CreatorConfig) Validate() error {
	if c.ControllerUUID == "" {
		return jujuerrors.NotValidf("empty ControllerUUID")
	}
	if err := c.ControllerModelUUID.Validate(); err != nil {
		return jujuerrors.Annotate(err, "controller model UUID")
	}
	if c.ControllerID == "" {
		return jujuerrors.NotValidf("empty ControllerID")
	}
	if c.DataDir == "" {
		return jujuerrors.NotValidf("empty DataDir")
	}
	if c.LogDir == "" {
		return jujuerrors.NotValidf("empty LogDir")
	}
	if c.ControllerExport == nil {
		return jujuerrors.NotValidf("nil ControllerExport")
	}
	if c.ModelServicesFor == nil {
		return jujuerrors.NotValidf("nil ModelServicesFor")
	}
	if c.ModelConfig == nil {
		return jujuerrors.NotValidf("nil ModelConfig")
	}
	if c.Controller == nil {
		return jujuerrors.NotValidf("nil Controller")
	}
	if c.ControllerNodes == nil {
		return jujuerrors.NotValidf("nil ControllerNodes")
	}
	if c.ControllerObjectStore == nil {
		return jujuerrors.NotValidf("nil ControllerObjectStore")
	}
	if c.ObjectStoreForModel == nil {
		return jujuerrors.NotValidf("nil ObjectStoreForModel")
	}
	if c.RecordSource == nil {
		return jujuerrors.NotValidf("nil RecordSource")
	}
	if c.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	return nil
}

// newCreator returns the archive creator for a creation request, bound
// to the source recorder the manifold injected for the controller's
// substrate.
func newCreator(cfg CreatorConfig) (*Creator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Creator{
		controllerUUID:        cfg.ControllerUUID,
		controllerModelUUID:   cfg.ControllerModelUUID,
		controllerID:          cfg.ControllerID,
		dataDir:               cfg.DataDir,
		logDir:                cfg.LogDir,
		controllerExport:      cfg.ControllerExport,
		modelServicesFor:      cfg.ModelServicesFor,
		modelConfig:           cfg.ModelConfig,
		controller:            cfg.Controller,
		controllerNodes:       cfg.ControllerNodes,
		controllerObjectStore: cfg.ControllerObjectStore,
		objectStoreForModel:   cfg.ObjectStoreForModel,
		recordSource:          cfg.RecordSource,
		clock:                 cfg.Clock,
		logger:                cfg.Logger,
	}, nil
}

// Create builds a fresh backup archive in a per-request temporary
// directory under the backup dir and returns its metadata and path.
// The archive contains the controller database export and one export
// per model, alongside the controller's data directory files.
//
// The caller owns the returned cleanup function, which removes the
// archive and its temporary directory; it must be called once the
// archive has been consumed, however the request ends.
//
// The controller database and each model database are exported at
// different points in time with no cross-database snapshot, so the
// archive is not a single point-in-time view of the whole controller.
// State that changes between the controller export and a given model
// export is not captured consistently. This is inherent to backing up
// multiple independent dqlite databases and is documented so recovery
// logic does not assume otherwise.
func (c *Creator) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	// The backup destination is resolved first because the database dumps
	// are staged as temporary files under it; staging keeps the archive from
	// holding every model's dump in memory at once.
	modelConfig, err := c.modelConfig.ModelConfig(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	backupDir := corebackups.BackupDirToUse(modelConfig.BackupDir())
	c.logger.Debugf(ctx, "creating backup in %q", backupDir)

	paths := corebackups.Paths{
		BackupDir: backupDir,
		DataDir:   c.dataDir,
		LogsDir:   c.logDir,
	}
	files, err := corebackups.GetFilesToBackUp("", &paths)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	controllerIDs, err := c.controllerNodes.GetControllerIDs(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}

	// Controller dump first, then one dump per registered model namespace.
	// Every registered model, including the controller model, owns a dqlite
	// database (namespace = its UUID) that is separate from the controller
	// database dumped above as controller.yaml. The controller model's
	// model-scoped data, its machines, units, applications, ... lives only
	// in that database, so its namespace is not skipped: this is not a
	// duplicate of the controller dump. Any error aborts Create: there are
	// no partial archives.
	var dumps []corebackups.NamedDump

	// The object inventory is derived from the typed export payloads:
	// every object referenced by a dump is streamed into the archive
	// from its namespace's object store.
	var inventory []domainexport.ObjectInventoryEntry

	controllerExport, err := c.controllerExport.Export(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	dumps = append(dumps, corebackups.NamedDump{
		Name:   "controller.yaml",
		Export: corebackups.YAMLDump(controllerExport),
	})
	controllerInventory, err := domainexport.ControllerObjectInventory(*controllerExport)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	inventory = append(inventory, controllerInventory...)

	modelUUIDs, err := c.controller.GetModelNamespaces(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	for _, modelUUID := range modelUUIDs {
		if err := ctx.Err(); err != nil {
			return nil, "", nil, err
		}
		modelServices, err := c.modelServicesFor(ctx, coremodel.UUID(modelUUID))
		if err != nil {
			return nil, "", nil, errors.Capture(err)
		}
		modelExport, err := modelServices.Export().Export(ctx)
		if err != nil {
			return nil, "", nil, errors.Capture(err)
		}
		dumps = append(dumps, corebackups.NamedDump{
			Name:   path.Join("models", modelUUID+".yaml"),
			Export: corebackups.YAMLDump(modelExport),
		})
		modelInventory, err := domainexport.ModelObjectInventory(*modelExport, modelUUID)
		if err != nil {
			return nil, "", nil, errors.Capture(err)
		}
		inventory = append(inventory, modelInventory...)
		c.logger.Tracef(ctx, "staged export for model %s", modelUUID)
	}

	// The dumps are staged as files inside the backup destination, so the
	// archive does not hold every model's dump in memory at once. The
	// staging is closed on return, so a failure leaves no partial dumps
	// behind.
	staging, err := corebackups.StageDumps(ctx, backupDir, dumps)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	// Close cleans up the staged dumps on return. Its error is only logged:
	// it must not mask the Create result.
	defer func() {
		if err := staging.Close(); err != nil {
			c.logger.Debugf(ctx, "cleaning up staged backup dumps: %v", err)
		}
	}()
	// Recovery rejects any dump larger than its read bound: fail the
	// backup now instead of producing an archive that cannot be
	// recovered.
	if name, size, err := staging.EntryExceedsLimit(recovery.MaxDumpSize); err != nil {
		return nil, "", nil, errors.Capture(err)
	} else if name != "" {
		return nil, "", nil, errors.Errorf(
			"database dump %q is %d bytes, exceeding the %d byte recovery limit: "+
				"the archive could never be recovered",
			name, size, recovery.MaxDumpSize)
	}
	expected, err := staging.Size()
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}

	for _, file := range files {
		// Lstat intentionally sizes the link itself instead of following it.
		fi, err := os.Lstat(file)
		if err != nil {
			// A file that cannot be stat'ed contributes nothing to the
			// estimate, which then understates what the archive needs.
			// That is not fatal here: archiving opens every one of these
			// files, so one that is really gone fails Create with a
			// clear error. Log it so an understated space check, and the
			// disk-full failure it can turn into, is diagnosable.
			c.logger.Warningf(ctx,
				"sizing backup file %q, excluded from the space estimate: %v",
				file, err)
			continue
		}
		expected += fi.Size()
	}

	// Each inventoried object is streamed into the archive through its
	// namespace's object store, identified by its exported full SHA-256.
	// The prefix lookup accepts a full hash and repairs missing local blobs
	// from other controller nodes for file-based stores.
	// Stores are resolved lazily and cached per namespace, so models
	// with no objects never touch the object store.
	stores := map[string]ReadObjectStore{
		database.ControllerNS: c.controllerObjectStore,
	}
	objectEntries := make([]corebackups.ObjectEntry, 0, len(inventory))
	for _, item := range inventory {
		expected += item.Size
		objectEntries = append(objectEntries, corebackups.ObjectEntry{
			Namespace: item.Namespace,
			SHA256:    item.SHA256,
			SHA384:    item.SHA384,
			Size:      item.Size,
			Source: func(ctx context.Context) (io.ReadCloser, error) {
				store, ok := stores[item.Namespace]
				if !ok {
					var err error
					store, err = c.objectStoreForModel(ctx, item.Namespace)
					if err != nil {
						return nil, errors.Capture(err)
					}
					stores[item.Namespace] = store
				}
				r, _, err := store.GetBySHA256Prefix(ctx, item.SHA256)
				return r, errors.Capture(err)
			},
		})
	}

	if err := corebackups.CheckSpaceFor(backupDir, expected); err != nil {
		return nil, "", nil, errors.Capture(err)
	}

	// The hostname is recorded for provenance only. If it cannot be resolved
	// the field is left empty; that does not make the backup unusable, so
	// the error is intentionally not fatal.
	hostname, _ := os.Hostname()

	meta := corebackups.NewMetadata(c.clock.Now())
	meta.Notes = notes
	meta.Origin = corebackups.Origin{
		Model:    c.controllerModelUUID.String(),
		Machine:  c.controllerID,
		Hostname: hostname,
		Version:  coreversion.Current,
	}
	meta.Controller = corebackups.ControllerMetadata{
		UUID:    c.controllerUUID,
		HANodes: int64(len(controllerIDs)),
	}
	meta.Instance = corebackups.InstanceMetadata{
		MachineID:         c.controllerID,
		MachineInstanceID: corebackups.UnknownString,
	}
	if err := c.recordSource(ctx, meta); err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	meta.DatabaseSizeBytes, err = databaseSize(ctx, c.dataDir)
	if err != nil {
		return nil, "", nil, errors.Errorf("recording source database capacity: %w", err)
	}

	// The archive is written into a per-request temporary directory
	// under the backup dir: its name is timestamp-based, so two
	// concurrent requests could collide in the backup dir itself.
	tmpDir, err := os.MkdirTemp(backupDir, "create-")
	if err != nil {
		return nil, "", nil, errors.Errorf("creating backup workspace: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	filename, err := corebackups.Create(ctx, meta, corebackups.CreateArgs{
		DestinationDir: tmpDir,
		DataDir:        c.dataDir,
		FilesToBackUp:  files,
		ObjectEntries:  objectEntries,
		DumpEntries:    staging.Entries(),
		Clock:          c.clock,
	})
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	c.logger.Infof(ctx, "created backup %q", filename)

	completed = true
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	return meta, filename, cleanup, nil
}

// machineSourceRecorder returns the source recorder for a machine
// controller: it records the controller machine's base in the archive
// metadata, which machine recovery requires. Kubernetes controllers have
// no machine and take their base from the archived controller charm. The
// base resolves through the controller model's services; the values are
// bound here rather than held by the creator.
func machineSourceRecorder(
	modelServicesFor ModelServicesForFunc,
	controllerModelUUID coremodel.UUID,
	controllerID string,
) RecordSourceFunc {
	return func(ctx context.Context, meta *corebackups.Metadata) error {
		services, err := modelServicesFor(ctx, controllerModelUUID)
		if err != nil {
			return errors.Errorf("recording controller base for recovery: %w", errors.Capture(err))
		}
		machineBase, err := services.Machine().GetMachineBase(ctx, machine.Name(controllerID))
		if err != nil {
			return errors.Errorf("recording controller base for recovery: %w", errors.Capture(err))
		}
		if machineBase.Empty() {
			return errors.New("recording controller base for recovery: controller machine base is empty")
		}
		meta.Origin.Base = machineBase.String()
		return nil
	}
}

// k8sSourceRecorder returns the source recorder for a Kubernetes
// controller: it overlays the running controller's provisioning facts
// onto the archive instance metadata. Kubernetes recovery requires them,
// so a reader that reports no facts fails the backup; the machine
// identifiers already recorded are preserved. The read is the only
// Kubernetes-specific dependency, so it is bound here rather than held
// by the creator.
func k8sSourceRecorder(
	read func(context.Context) (*corebackups.InstanceMetadata, error),
) RecordSourceFunc {
	return func(ctx context.Context, meta *corebackups.Metadata) error {
		instance, err := read(ctx)
		if err == nil && instance == nil {
			err = errors.New("no provisioning facts recorded")
		}
		if err != nil {
			return errors.Errorf("recording Kubernetes provisioning: %w", err)
		}
		meta.Instance.AgentImage = instance.AgentImage
		meta.Instance.CharmImage = instance.CharmImage
		meta.Instance.Architecture = instance.Architecture
		meta.Instance.StorageClass = instance.StorageClass
		meta.Instance.StorageCapacityMiB = instance.StorageCapacityMiB
		meta.Instance.ServiceType = instance.ServiceType
		meta.Instance.ExternalName = instance.ExternalName
		meta.Instance.ExternalIPs = instance.ExternalIPs
		meta.Instance.LoadBalancerIP = instance.LoadBalancerIP
		meta.Instance.ServiceAnnotations = instance.ServiceAnnotations
		return nil
	}
}
