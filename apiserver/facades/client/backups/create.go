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
	"github.com/juju/juju/rpc/params"
)

// Create is the API method that requests juju to create a new backup
// of its state. The archive contains the controller database export and one
// export per model, alongside the controller's data directory files.
func (a *API) Create(ctx context.Context, args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	// Creating a backup requires superuser access to the controller. This is
	// the same gate Juju 3.6 applies to the Create method.
	if err := a.authorizer.HasPermission(
		ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID),
	); err != nil {
		return params.BackupsMetadataResult{}, err
	}

	// The backup destination is resolved first because the database dumps
	// are staged as temporary files under it; staging keeps the archive from
	// holding every model's dump in memory at once.
	modelConfig, err := a.modelConfig.ModelConfig(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	backupDir := corebackups.BackupDirToUse(modelConfig.BackupDir())

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
		return params.BackupsMetadataResult{}, err
	}
	dumps = append(dumps, corebackups.NamedDump{
		Name:   "controller.yaml",
		Export: corebackups.YAMLDump(controllerExport),
	})

	modelUUIDs, err := a.controller.GetModelNamespaces(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	for _, modelUUID := range modelUUIDs {
		modelServices, err := a.modelServicesFor(ctx, coremodel.UUID(modelUUID))
		if err != nil {
			return params.BackupsMetadataResult{}, err
		}
		modelExport, err := modelServices.Export().Export(ctx)
		if err != nil {
			return params.BackupsMetadataResult{}, err
		}
		dumps = append(dumps, corebackups.NamedDump{
			Name:   path.Join("models", modelUUID+".yaml"),
			Export: corebackups.YAMLDump(modelExport),
		})
	}

	// The dumps are staged as files inside the backup destination, so the
	// archive does not hold every model's dump in memory at once. The
	// staging is closed on return, so a failure leaves no partial dumps
	// behind.
	staging, err := corebackups.StageDumps(ctx, backupDir, dumps)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	defer staging.Close()
	expected := staging.Size()

	paths := corebackups.Paths{
		BackupDir: backupDir,
		DataDir:   a.dataDir,
		LogsDir:   a.logDir,
	}
	files, err := corebackups.GetFilesToBackUp("", &paths)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	for _, file := range files {
		if fi, err := os.Lstat(file); err == nil {
			expected += fi.Size()
		}
	}

	if err := corebackups.CheckSpaceFor(backupDir, expected); err != nil {
		return params.BackupsMetadataResult{}, err
	}

	// The hostname is recorded for provenance only. If it cannot be resolved
	// the field is left empty; that does not make the backup unusable, so
	// the error is intentionally not fatal.
	hostname, _ := os.Hostname()

	controllerIDs, err := a.controllerNodes.GetControllerIDs(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}

	meta := corebackups.NewMetadata(a.clock.Now())
	meta.Notes = args.Notes
	meta.Origin = corebackups.Origin{
		Model:    a.controllerModelUID.String(),
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
		return params.BackupsMetadataResult{}, err
	}

	return params.CreateResult(meta, filename), nil
}
