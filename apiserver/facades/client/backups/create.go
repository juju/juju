// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"os"
	"path"

	"github.com/juju/names/v6"

	corebackups "github.com/juju/juju/core/backups"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Create is the API method that requests juju to create a new backup
// of its state. The archive contains the controller database export and one
// export per model, alongside the controller's data directory files.
//
// Only args.Notes is honored. args.NoDownload is accepted for client
// compatibility but has no effect here: the archive is always written to the
// controller's backup directory, and the download endpoint is not wired yet.
//
// The controller database and each model database are exported at different
// points in time with no cross-database snapshot, so the archive is not a
// single point-in-time view of the whole controller. State that changes
// between the controller export and a given model export is not captured
// consistently. This is inherent to backing up multiple independent dqlite
// databases and is documented so restore logic does not assume otherwise.
func (a *API) Create(ctx context.Context, args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	// Creating a backup requires superuser access to the controller. This is
	// the same gate Juju 3.6 applies to the Create method.
	if err := a.authorizer.HasPermission(
		ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID),
	); err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	// The backup destination is resolved first because the database dumps
	// are staged as temporary files under it; staging keeps the archive from
	// holding every model's dump in memory at once.
	modelConfig, err := a.modelConfig.ModelConfig(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	backupDir := corebackups.BackupDirToUse(modelConfig.BackupDir())
	a.logger.Debugf(ctx, "creating backup in %q", backupDir)

	// Controller dump first, then one dump per registered model namespace.
	// Every registered model, including the controller model, owns a dqlite
	// database (namespace = its UUID) that is separate from the controller
	// database dumped above as controller.yaml. The controller model's
	// model-scoped data, its machines, units, applications, ... lives only
	// in that database, so its namespace is not skipped: this is not a
	// duplicate of the controller dump. Any error aborts Create: there are
	// no partial archives.
	dumps := []corebackups.NamedDump{}

	controllerExport, err := a.controllerExport.Export(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	dumps = append(dumps, corebackups.NamedDump{
		Name:   "controller.yaml",
		Export: corebackups.YAMLDump(controllerExport),
	})

	modelUUIDs, err := a.controller.GetModelNamespaces(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	for _, modelUUID := range modelUUIDs {
		if err := ctx.Err(); err != nil {
			return params.BackupsMetadataResult{}, err
		}
		modelServices, err := a.modelServicesFor(ctx, coremodel.UUID(modelUUID))
		if err != nil {
			return params.BackupsMetadataResult{}, errors.Capture(err)
		}
		modelExport, err := modelServices.Export().Export(ctx)
		if err != nil {
			return params.BackupsMetadataResult{}, errors.Capture(err)
		}
		dumps = append(dumps, corebackups.NamedDump{
			Name:   path.Join("models", modelUUID+".yaml"),
			Export: corebackups.YAMLDump(modelExport),
		})
		a.logger.Tracef(ctx, "staged export for model %s", modelUUID)
	}

	// The dumps are staged as files inside the backup destination, so the
	// archive does not hold every model's dump in memory at once. The
	// staging is closed on return, so a failure leaves no partial dumps
	// behind.
	staging, err := corebackups.StageDumps(ctx, backupDir, dumps)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	// Close cleans up the staged dumps on return. Its error is only logged:
	// it must not mask the Create result.
	defer func() {
		if err := staging.Close(); err != nil {
			a.logger.Debugf(ctx, "cleaning up staged backup dumps: %v", err)
		}
	}()
	expected, err := staging.Size()
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	paths := corebackups.Paths{
		BackupDir: backupDir,
		DataDir:   a.dataDir,
		LogsDir:   a.logDir,
	}
	files, err := corebackups.GetFilesToBackUp("", &paths)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	for _, file := range files {
		fi, err := os.Lstat(file)
		if err != nil {
			// A file that cannot be stat'ed contributes nothing to the
			// estimate, which then understates what the archive needs.
			// That is not fatal here: bundling opens every one of these
			// files, so one that is really gone fails Create with a
			// clear error. Log it so an understated space check, and the
			// disk-full failure it can turn into, is diagnosable.
			a.logger.Warningf(ctx,
				"sizing backup file %q, excluded from the space estimate: %v",
				file, err)
			continue
		}
		expected += fi.Size()
	}

	if err := corebackups.CheckSpaceFor(backupDir, expected); err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	// The hostname is recorded for provenance only. If it cannot be resolved
	// the field is left empty; that does not make the backup unusable, so
	// the error is intentionally not fatal.
	hostname, _ := os.Hostname()

	controllerIDs, err := a.controllerNodes.GetControllerIDs(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	meta := corebackups.NewMetadata(a.clock.Now())
	meta.Notes = args.Notes
	meta.Origin = corebackups.Origin{
		Model:    a.controllerModelUUID.String(),
		Machine:  a.machineID,
		Hostname: hostname,
		Version:  coreversion.Current,
		// Base is not resolved: there is no verified cheap machine-base
		// lookup path in this facade and the field is informational only.
		Base: "",
	}
	meta.Controller = corebackups.ControllerMetadata{
		UUID:      a.controllerUUID,
		MachineID: a.machineID,
		// TODO(backups): resolve the controller machine's cloud instance id
		// via the machine service (GetInstanceIDByMachineName) and record it
		// here instead of the unknown placeholder.
		MachineInstanceID: corebackups.UnknownString,
		HANodes:           int64(len(controllerIDs)),
	}

	filename, err := corebackups.Create(meta, corebackups.CreateArgs{
		DestinationDir: backupDir,
		FilesToBackUp:  files,
		DumpEntries:    staging.Entries(),
		Clock:          a.clock,
	})
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}
	a.logger.Infof(ctx, "created backup %q", filename)

	return params.CreateResult(meta, filename), nil
}
