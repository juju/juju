// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"context"
	"os"
	"path"

	"github.com/juju/names/v6"
	"gopkg.in/yaml.v3"

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

	// Controller dump first, then one dump per registered model namespace.
	// Any error aborts Create: there are no partial archives.
	entries := []corebackups.DumpEntry{}
	var expected int64

	controllerExport, err := a.controllerExport.Export(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	controllerYAML, err := yaml.Marshal(controllerExport)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	expected += int64(len(controllerYAML))
	entries = append(entries, corebackups.DumpEntry{
		Name:   "controller.yaml",
		Reader: bytes.NewReader(controllerYAML),
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
		modelYAML, err := yaml.Marshal(modelExport)
		if err != nil {
			return params.BackupsMetadataResult{}, err
		}
		expected += int64(len(modelYAML))
		entries = append(entries, corebackups.DumpEntry{
			Name:   path.Join("models", modelUUID+".yaml"),
			Reader: bytes.NewReader(modelYAML),
		})
	}

	modelConfig, err := a.modelConfig.ModelConfig(ctx)
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}
	backupDir := corebackups.BackupDirToUse(modelConfig.BackupDir())

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
		UUID:              a.controllerUUID,
		MachineID:         a.machineID,
		MachineInstanceID: corebackups.UnknownString,
		HANodes:           int64(len(controllerIDs)),
	}

	filename, err := corebackups.Create(meta, corebackups.CreateArgs{
		DestinationDir: backupDir,
		FilesToBackUp:  files,
		DumpEntries:    entries,
		Clock:          a.clock,
	})
	if err != nil {
		return params.BackupsMetadataResult{}, err
	}

	return params.CreateResult(meta, filename), nil
}
