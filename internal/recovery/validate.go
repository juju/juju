// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"bytes"
	"context"
	"os"
	"strconv"

	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	domainrecovery "github.com/juju/juju/domain/recovery"
	recoveryservice "github.com/juju/juju/domain/recovery/service"
	"github.com/juju/juju/internal/errors"
)

// ValidateArchive reads the backup archive at archivePath and returns
// its validated summary. When expectedSHA256 is not empty it must match
// the archive's SHA-256 checksum (hex): the expected value always comes
// from the operator, never from inside the archive.
//
// Validation is offline and runs before anything is provisioned. Model
// dumps are staged in temporary files that are removed before returning.
func ValidateArchive(ctx context.Context, logger corelogger.Logger, archivePath, expectedSHA256 string) (*domainrecovery.ArchiveInfo, error) {
	contents, info, err := ReadArchive(ctx, logger, archivePath, expectedSHA256)
	if err != nil {
		return nil, err
	}
	if err := contents.Close(); err != nil {
		return nil, errors.Errorf("cleaning up staged recovery dumps: %w", err)
	}
	return info, nil
}

// ReadArchive reads and validates the backup archive at archivePath and
// returns both the extracted contents — metadata and database dumps —
// and the validated summary. The agent-side recovery stage consumes the
// contents; the offline preflight consumes the summary alone via
// [ValidateArchive]. The caller must Close the contents once consumed.
func ReadArchive(ctx context.Context, logger corelogger.Logger, archivePath, expectedSHA256 string) (*ArchiveContents, *domainrecovery.ArchiveInfo, error) {
	contents, checksum, size, err := readArchive(ctx, logger, archivePath, expectedSHA256)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	info, err := parseArchiveInfo(ctx, logger, contents)
	if err != nil {
		_ = contents.Close()
		return nil, nil, errors.Capture(err)
	}
	info.Checksum = checksum
	info.Size = size
	return contents, info, nil
}

// parseArchiveInfo builds the archive summary from extracted contents:
// metadata, controller dump and the model dump inventory.
func parseArchiveInfo(ctx context.Context, logger corelogger.Logger, contents *ArchiveContents) (*domainrecovery.ArchiveInfo, error) {
	meta, err := corebackups.NewMetadataJSONReader(bytes.NewReader(contents.Metadata))
	if err != nil {
		return nil, errors.Errorf("parsing %s: %w", metadataPath, err)
	}
	if meta.Origin.Version == corebackups.UnknownVersion ||
		meta.Origin.Version == (semversion.Number{}) {
		return nil, errors.Errorf("%s does not record the source agent version", metadataPath)
	}

	payload, err := recoveryservice.DecodeControllerPreflight(contents.ControllerDump)
	if err != nil {
		return nil, errors.Errorf("parsing %s: %w", controllerDumpPath, err)
	}

	info, err := buildArchiveInfo(ctx, logger, meta, payload, contents.ModelDumps)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return info, nil
}

// buildArchiveInfo resolves clouds and model types from the controller
// dump and cross-checks the model dump inventory against the model table.
func buildArchiveInfo(ctx context.Context, logger corelogger.Logger, meta *corebackups.Metadata, payload *recoveryservice.ControllerPreflightDump, modelDumps map[string]string) (*domainrecovery.ArchiveInfo, error) {
	cloudTypes := make(map[int64]string)
	for _, ct := range payload.CloudType {
		if ct.ID != nil {
			cloudTypes[*ct.ID] = ct.Type
		}
	}
	modelTypes := make(map[int64]string)
	for _, mt := range payload.ModelType {
		if mt.ID != nil {
			modelTypes[*mt.ID] = mt.Type
		}
	}
	cloudRegions := make(map[string]string, len(payload.CloudRegion))
	for _, cr := range payload.CloudRegion {
		cloudRegions[cr.UUID] = cr.Name
	}
	type cloudRef struct {
		name     string
		provider string
	}
	clouds := make(map[string]cloudRef)
	for _, cl := range payload.Cloud {
		clouds[cl.UUID] = cloudRef{name: cl.Name, provider: cloudTypes[cl.CloudTypeID]}
	}

	info := &domainrecovery.ArchiveInfo{
		// Only machine allocation requires a base; Kubernetes has none.
		SourceBase:          meta.Origin.Base,
		SourceArch:          meta.Origin.Arch,
		MachineName:         meta.Origin.Machine,
		AgentVersion:        meta.Origin.Version,
		ControllerUUID:      meta.Controller.UUID,
		ControllerModelUUID: meta.Origin.Model,
		HANodes:             meta.Controller.HANodes,
		ControllerConfig:    make(map[string]string, len(payload.ControllerConfig)),
	}
	for _, cc := range payload.ControllerConfig {
		// A NULL row value records no setting at all; reporting it as an
		// empty string would override the config default with a blank.
		if cc.Value == nil {
			continue
		}
		info.ControllerConfig[cc.Key] = *cc.Value
		if cc.Key == controller.ControllerName {
			info.ControllerName = *cc.Value
		}
	}

	if len(payload.Controller) == 0 {
		return nil, errors.Errorf("%s records no controller row", controllerDumpPath)
	}
	controllerRow := payload.Controller[0]
	if controllerRow.CaCert == nil || controllerRow.CaPrivateKey == nil {
		return nil, errors.Errorf("%s records no controller CA material", controllerDumpPath)
	}
	info.CACert = *controllerRow.CaCert
	info.CAPrivateKey = *controllerRow.CaPrivateKey
	if controllerRow.ModelUUID != "" &&
		info.ControllerModelUUID != controllerRow.ModelUUID {
		return nil, errors.Errorf(
			"controller model mismatch: metadata records %q, controller row records %q",
			info.ControllerModelUUID, controllerRow.ModelUUID)
	}
	// The manifest and the dump must agree on the controller's identity:
	// the recovered controller adopts the manifest's uuid, so a divergent
	// dump row means the archive is internally inconsistent.
	if controllerRow.UUID != "" && controllerRow.UUID != info.ControllerUUID {
		return nil, errors.Errorf(
			"controller uuid mismatch: metadata records %q, controller row records %q",
			info.ControllerUUID, controllerRow.UUID)
	}
	// api-port and ssh-server-port are derived columns of the controller
	// config view rather than rows of the controller_config table, so the
	// dump stores them outside it.
	if controllerRow.ApiPort != nil && *controllerRow.ApiPort != "" {
		info.ControllerConfig[controller.APIPort] = *controllerRow.ApiPort
	}
	if len(payload.ControllerSSHServerPort) > 0 && payload.ControllerSSHServerPort[0].Port > 0 {
		if _, ok := info.ControllerConfig[controller.SSHServerPort]; !ok {
			info.ControllerConfig[controller.SSHServerPort] = strconv.FormatInt(
				payload.ControllerSSHServerPort[0].Port, 10)
		}
	}

	if len(payload.Model) == 0 {
		return nil, errors.Errorf("%s records no models", controllerDumpPath)
	}
	seenModels := make(map[string]struct{}, len(payload.Model))
	for _, m := range payload.Model {
		if _, dup := seenModels[m.UUID]; dup {
			return nil, errors.Errorf("dump records duplicate model uuid %q", m.UUID)
		}
		seenModels[m.UUID] = struct{}{}
		cloud, ok := clouds[m.CloudUUID]
		if !ok {
			return nil, errors.Errorf("model %q references unknown cloud %q", m.Name, m.CloudUUID)
		}
		mi := domainrecovery.ModelInfo{
			UUID:      m.UUID,
			Name:      m.Name,
			ModelType: modelTypes[m.ModelTypeID],
			CloudName: cloud.name,
			CloudType: cloud.provider,
		}
		if mi.ModelType == "" {
			return nil, errors.Errorf("model %q has unknown model type", m.Name)
		}
		if mi.CloudType == "" {
			return nil, errors.Errorf("model %q has unknown cloud type", m.Name)
		}
		info.Models = append(info.Models, mi)
		if m.UUID == info.ControllerModelUUID {
			info.CloudName = mi.CloudName
			info.CloudType = mi.CloudType
			if m.CloudRegionUUID != nil && *m.CloudRegionUUID != "" {
				regionName, ok := cloudRegions[*m.CloudRegionUUID]
				if !ok {
					return nil, errors.Errorf(
						"controller model %q references unknown cloud region %q",
						m.Name, *m.CloudRegionUUID)
				}
				info.CloudRegion = regionName
			}
		}
	}
	if info.CloudType == "" {
		return nil, errors.Errorf("controller model %q not found in %s", info.ControllerModelUUID, controllerDumpPath)
	}

	// The dump inventory must match the model table exactly: a dump
	// without a model row, or a model without its dump, means the
	// archive is incomplete.
	for _, mi := range info.Models {
		if _, ok := modelDumps[mi.UUID]; !ok {
			return nil, errors.Errorf("archive is missing the database dump for model %q (%s)", mi.Name, mi.UUID)
		}
	}
	for uuid := range modelDumps {
		if _, ok := seenModels[uuid]; !ok {
			return nil, errors.Errorf("archive contains a database dump for unknown model %q", uuid)
		}
	}

	// Fill the Kubernetes workload inventory from the model dumps: it
	// powers the read-only substrate check, which reports missing
	// workload objects instead of silently recreating them at the first
	// reconcile. The controller model is skipped: its namespace is
	// disposable bootstrap output, not surviving substrate.
	for i := range info.Models {
		mi := &info.Models[i]
		if mi.ModelType != "caas" || mi.UUID == info.ControllerModelUUID {
			continue
		}
		apps, err := readModelInventory(ctx, logger, modelDumps[mi.UUID])
		if err != nil {
			return nil, errors.Errorf("model %q: %w", mi.Name, err)
		}
		mi.Applications = apps
	}

	// The controller model dump carries the provisioning inputs a
	// replacement controller must reuse: its model config (which holds
	// provider settings such as an LXD project), its constraints and the
	// storage pools it was bootstrapped with.
	controllerModel, err := readControllerModelProvisioning(ctx, modelDumps[info.ControllerModelUUID])
	if err != nil {
		return nil, errors.Errorf("controller model %q: %w", info.ControllerModelUUID, err)
	}
	info.ControllerModelConfig = controllerModel.Config
	info.ModelConstraints = controllerModel.ModelConstraints
	info.ControllerConstraints = controllerModel.ControllerConstraints
	info.StoragePools = controllerModel.StoragePools
	// Kubernetes agents do not necessarily have a machine SSH identity;
	// every supported machine-controller backup does.
	if info.CloudType != "kubernetes" &&
		(controllerRow.SystemIdentity == nil || *controllerRow.SystemIdentity == "") {
		return nil, errors.Errorf("%s records no controller SSH identity", controllerDumpPath)
	}
	if controllerRow.SystemIdentity != nil {
		info.SystemIdentity = *controllerRow.SystemIdentity
	}
	return info, nil
}

// readModelInventory opens a staged model dump and streams it through the
// domain service's Kubernetes substrate inventory decoder.
func readModelInventory(ctx context.Context, logger corelogger.Logger, filename string) ([]domainrecovery.ApplicationInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Errorf("opening dump: %w", err)
	}
	defer func() { _ = file.Close() }()
	return recoveryservice.DecodeModelInventory(ctx, logger, contextReader{ctx: ctx, reader: file})
}

// readControllerModelProvisioning opens the staged controller model dump and
// streams it through the domain service's provisioning-inputs decoder.
func readControllerModelProvisioning(ctx context.Context, filename string) (recoveryservice.ControllerModelProvisioning, error) {
	file, err := os.Open(filename)
	if err != nil {
		return recoveryservice.ControllerModelProvisioning{}, errors.Errorf("opening dump: %w", err)
	}
	defer func() { _ = file.Close() }()
	return recoveryservice.DecodeControllerModelProvisioning(contextReader{ctx: ctx, reader: file})
}
