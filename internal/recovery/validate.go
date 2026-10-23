// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"bytes"
	"context"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	domainlife "github.com/juju/juju/domain/life"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// controllerDumpPayload mirrors the fields of the controller database
// dump that preflight validation needs. Unknown fields are ignored; the
// full decode happens agent-side during the load stage.
type controllerDumpPayload struct {
	Controller []struct {
		UUID         string  `yaml:"uuid"`
		ModelUUID    string  `yaml:"model_uuid"`
		CACert       *string `yaml:"ca_cert"`
		CAPrivateKey *string `yaml:"ca_private_key"`
	} `yaml:"controller"`
	Cloud []struct {
		UUID        string `yaml:"uuid"`
		Name        string `yaml:"name"`
		CloudTypeID int64  `yaml:"cloud_type_id"`
	} `yaml:"cloud"`
	CloudType []struct {
		ID   *int64 `yaml:"id"`
		Type string `yaml:"type"`
	} `yaml:"cloud_type"`
	Model []struct {
		UUID        string `yaml:"uuid"`
		Name        string `yaml:"name"`
		CloudUUID   string `yaml:"cloud_uuid"`
		ModelTypeID int64  `yaml:"model_type_id"`
	} `yaml:"model"`
	ModelType []struct {
		ID   *int64 `yaml:"id"`
		Type string `yaml:"type"`
	} `yaml:"model_type"`
	ControllerConfig []struct {
		Key   string `yaml:"key"`
		Value string `yaml:"value"`
	} `yaml:"controller_config"`
}

// controllerDumpEnvelope is the top-level shape of controller.yaml. The
// envelope version is intentionally not parsed here: the agent-version
// gate implies matching dump formats, and the loader validates them.
type controllerDumpEnvelope struct {
	Payload controllerDumpPayload `yaml:"payload"`
}

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

	var dump controllerDumpEnvelope
	if err := yaml.Unmarshal(contents.ControllerDump, &dump); err != nil {
		return nil, errors.Errorf("parsing %s: %w", controllerDumpPath, err)
	}

	info, err := buildArchiveInfo(ctx, logger, meta, &dump.Payload, contents.ModelDumps)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if meta.Finished != nil {
		info.BackupFinished = *meta.Finished
	}
	return info, nil
}

// buildArchiveInfo resolves clouds and model types from the controller
// dump and cross-checks the model dump inventory against the model table.
func buildArchiveInfo(ctx context.Context, logger corelogger.Logger, meta *corebackups.Metadata, payload *controllerDumpPayload, modelDumps map[string]string) (*domainrecovery.ArchiveInfo, error) {
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
	type cloudRef struct {
		name     string
		provider string
	}
	clouds := make(map[string]cloudRef)
	for _, cl := range payload.Cloud {
		clouds[cl.UUID] = cloudRef{name: cl.Name, provider: cloudTypes[cl.CloudTypeID]}
	}

	info := &domainrecovery.ArchiveInfo{
		AgentVersion:        meta.Origin.Version,
		ControllerUUID:      meta.Controller.UUID,
		ControllerModelUUID: meta.Origin.Model,
		HANodes:             meta.Controller.HANodes,
	}
	for _, cc := range payload.ControllerConfig {
		if cc.Key == controller.ControllerName {
			info.ControllerName = cc.Value
		}
	}

	if len(payload.Controller) == 0 {
		return nil, errors.Errorf("%s records no controller row", controllerDumpPath)
	}
	controller := payload.Controller[0]
	if controller.CACert == nil || controller.CAPrivateKey == nil {
		return nil, errors.Errorf("%s records no controller CA material", controllerDumpPath)
	}
	info.CACert = *controller.CACert
	info.CAPrivateKey = *controller.CAPrivateKey
	if controller.ModelUUID != "" &&
		info.ControllerModelUUID != controller.ModelUUID {
		return nil, errors.Errorf(
			"controller model mismatch: metadata records %q, controller row records %q",
			info.ControllerModelUUID, controller.ModelUUID)
	}
	// The manifest and the dump must agree on the controller's identity:
	// the recovered controller adopts the manifest's uuid, so a divergent
	// dump row means the archive is internally inconsistent.
	if controller.UUID != "" && controller.UUID != info.ControllerUUID {
		return nil, errors.Errorf(
			"controller uuid mismatch: metadata records %q, controller row records %q",
			info.ControllerUUID, controller.UUID)
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
		apps, err := inventoryFromModelDump(ctx, logger, modelDumps[mi.UUID])
		if err != nil {
			return nil, errors.Errorf("model %q: %w", mi.Name, err)
		}
		mi.Applications = apps
	}
	return info, nil
}

// modelDumpInventory mirrors the model-dump tables the substrate
// inventory reads. Unknown tables and fields are ignored.
//
// Life filtering excludes only dead rows: dying entities still exist
// in the cluster, so they count as surviving substrate the preflight
// must account for.
type modelDumpInventory struct {
	Application []struct {
		UUID   string          `yaml:"uuid"`
		Name   string          `yaml:"name"`
		LifeID domainlife.Life `yaml:"life_id"`
	} `yaml:"application"`
	Unit []struct {
		Name            string          `yaml:"name"`
		LifeID          domainlife.Life `yaml:"life_id"`
		ApplicationUUID string          `yaml:"application_uuid"`
		NetNodeUUID     string          `yaml:"net_node_uuid"`
	} `yaml:"unit"`
	StorageFilesystem []struct {
		UUID       string          `yaml:"uuid"`
		ProviderID string          `yaml:"provider_id"`
		LifeID     domainlife.Life `yaml:"life_id"`
	} `yaml:"storage_filesystem"`
	StorageFilesystemAttachment []struct {
		StorageFilesystemUUID string          `yaml:"storage_filesystem_uuid"`
		NetNodeUUID           string          `yaml:"net_node_uuid"`
		LifeID                domainlife.Life `yaml:"life_id"`
	} `yaml:"storage_filesystem_attachment"`
}

// inventoryFromModelDump extracts the surviving-substrate inventory of
// one Kubernetes model: its alive applications, their alive units, and
// the persistent volume claim names recorded by alive storage
// filesystems.
// A claim name is attributed to the application whose unit attaches it.
func inventoryFromModelDump(ctx context.Context, logger corelogger.Logger, filename string) ([]domainrecovery.ApplicationInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Errorf("opening dump: %w", err)
	}
	defer func() { _ = file.Close() }()

	var envelope struct {
		Payload modelDumpInventory `yaml:"payload"`
	}
	if err := yaml.NewDecoder(contextReader{ctx: ctx, reader: file}).Decode(&envelope); err != nil {
		return nil, errors.Errorf("decoding dump: %w", err)
	}
	inv := envelope.Payload

	// unitAppByNode maps a unit's net node to the unit's application,
	// resolving which application each attached volume belongs to.
	unitAppByNode := make(map[string]string, len(inv.Unit))
	for _, u := range inv.Unit {
		if u.LifeID == domainlife.Dead {
			continue
		}
		if u.NetNodeUUID == "" {
			logger.Debugf(ctx, "unit %q has no net node, its volume attachments are not attributed", u.Name)
			continue
		}
		unitAppByNode[u.NetNodeUUID] = u.ApplicationUUID
	}
	// provider_id is the persistent volume claim name: its unique index
	// is how the running controller re-attaches volumes it finds in the
	// cluster, so it is the name the substrate check must verify.
	filesystemClaim := make(map[string]string, len(inv.StorageFilesystem))
	for _, fs := range inv.StorageFilesystem {
		if fs.ProviderID == "" || fs.LifeID == domainlife.Dead {
			continue
		}
		filesystemClaim[fs.UUID] = fs.ProviderID
	}
	claimsByApp := make(map[string][]string)
	for _, att := range inv.StorageFilesystemAttachment {
		if att.LifeID == domainlife.Dead {
			continue
		}
		claim, ok := filesystemClaim[att.StorageFilesystemUUID]
		if !ok {
			logger.Debugf(ctx, "attachment references unknown filesystem %q, skipped", att.StorageFilesystemUUID)
			continue
		}
		app, ok := unitAppByNode[att.NetNodeUUID]
		if !ok || app == "" {
			logger.Debugf(ctx, "attachment of claim %q has no living unit to attribute it to, skipped", claim)
			continue
		}
		claimsByApp[app] = append(claimsByApp[app], claim)
	}

	inventory := make([]domainrecovery.ApplicationInfo, 0, len(inv.Application))
	for _, app := range inv.Application {
		// Dead applications are recorded in the recovery summary as
		// pending removals; their substrate is not checked.
		if app.LifeID == domainlife.Dead {
			continue
		}
		// An alive application without its identity is not a row the
		// dump format produces; the dump is corrupt.
		if app.UUID == "" || app.Name == "" {
			return nil, errors.Errorf("dump records an alive application with an empty uuid or name")
		}
		info := domainrecovery.ApplicationInfo{
			UUID:                  app.UUID,
			Name:                  app.Name,
			FilesystemProviderIDs: claimsByApp[app.UUID],
		}
		for _, u := range inv.Unit {
			if u.ApplicationUUID == app.UUID && u.LifeID != domainlife.Dead {
				info.Units = append(info.Units, u.Name)
			}
		}
		inventory = append(inventory, info)
	}
	return inventory, nil
}
