// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"os"
	"path"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	corebackups "github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Create is the API method that requests juju to create a new backup
// of its state. As of 4.1 the RPC method no longer creates anything:
// backup creation and download happen in a single request against the
// controller's backups HTTP endpoint, so this method only remains for
// older clients, which are rejected with a clear upgrade error before
// any archive is created.
func (a *API) Create(ctx context.Context, args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	// Creating a backup requires superuser access to the controller. This
	// mirrors the long-standing access gate for backup creation.
	if err := a.authorizer.HasPermission(
		ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID),
	); err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	return params.BackupsMetadataResult{}, errors.Errorf(
		"create-backup from clients older than 4.1 is not supported; use a 4.1 or newer client",
	).Add(coreerrors.NotSupported)
}

// Creator creates backup archives on demand. It is used by the
// controller's backups HTTP endpoint, which streams the archive to the
// client in the same request.
type Creator struct {
	controllerUUID      string
	controllerModelUUID coremodel.UUID
	machineID           string
	dataDir             string
	logDir              string

	controllerExport ControllerExportService
	modelServicesFor ModelServicesForFunc
	modelConfig      ModelConfigService
	controller       ControllerModelLister
	controllerNodes  ControllerNodeLister
	clock            clock.Clock
	logger           corelogger.Logger
}

// NewCreator creates a new backup archive creator.
func NewCreator(
	machineTag names.Tag,
	controllerUUID string,
	controllerModelUUID coremodel.UUID,
	dataDir, logDir string,
	controllerExport ControllerExportService,
	modelServicesFor ModelServicesForFunc,
	modelConfig ModelConfigService,
	controller ControllerModelLister,
	controllerNodes ControllerNodeLister,
	clock clock.Clock,
	logger corelogger.Logger,
) (*Creator, error) {
	return &Creator{
		controllerUUID:      controllerUUID,
		controllerModelUUID: controllerModelUUID,
		machineID:           machineTag.Id(),
		dataDir:             dataDir,
		logDir:              logDir,
		controllerExport:    controllerExport,
		modelServicesFor:    modelServicesFor,
		modelConfig:         modelConfig,
		controller:          controller,
		controllerNodes:     controllerNodes,
		clock:               clock,
		logger:              logger,
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
// multiple independent dqlite databases and is documented so restore
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

	controllerExport, err := c.controllerExport.Export(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	dumps = append(dumps, corebackups.NamedDump{
		Name:   "controller.yaml",
		Export: corebackups.YAMLDump(controllerExport),
	})

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
			// That is not fatal here: bundling opens every one of these
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
		Machine:  c.machineID,
		Hostname: hostname,
		Version:  coreversion.Current,
		// TODO(backups): resolve the controller machine's base when a cheap,
		// verified lookup path is available to this facade.
		Base: "",
	}
	meta.Controller = corebackups.ControllerMetadata{
		UUID:              c.controllerUUID,
		MachineID:         c.machineID,
		MachineInstanceID: corebackups.UnknownString,
		HANodes:           int64(len(controllerIDs)),
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

	filename, err := corebackups.Create(meta, corebackups.CreateArgs{
		DestinationDir: tmpDir,
		FilesToBackUp:  files,
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
