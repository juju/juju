// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"bytes"
	"context"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// controllerDumpPayload mirrors the fields of the controller database
// dump that preflight validation needs. Unknown fields are ignored; the
// full decode happens agent-side during the load stage.
type controllerDumpPayload struct {
	Controller []struct {
		UUID           string  `yaml:"uuid"`
		ModelUUID      string  `yaml:"model_uuid"`
		CACert         *string `yaml:"ca_cert"`
		CAPrivateKey   *string `yaml:"ca_private_key"`
		SystemIdentity *string `yaml:"system_identity"`
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
// Validation is offline and read-only: it runs on the bootstrap client
// before anything is provisioned.
func ValidateArchive(ctx context.Context, archivePath, expectedSHA256 string) (*domainrecovery.ArchiveInfo, error) {
	_, info, err := ReadArchive(ctx, archivePath, expectedSHA256)
	return info, err
}

// ReadArchive reads and validates the backup archive at archivePath and
// returns both the extracted contents — metadata and database dumps —
// and the validated summary. The agent-side recovery stage consumes the
// contents; the offline preflight consumes the summary alone via
// [ValidateArchive].
func ReadArchive(ctx context.Context, archivePath, expectedSHA256 string) (*ArchiveContents, *domainrecovery.ArchiveInfo, error) {
	contents, checksum, size, err := readArchive(ctx, archivePath, expectedSHA256)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	info, err := parseArchiveInfo(contents)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	info.Checksum = checksum
	info.Size = size
	return contents, info, nil
}

// parseArchiveInfo builds the archive summary from extracted contents:
// metadata, controller dump and the model dump inventory.
func parseArchiveInfo(contents *ArchiveContents) (*domainrecovery.ArchiveInfo, error) {
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

	info, err := buildArchiveInfo(meta, &dump.Payload, contents.ModelDumps)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return info, nil
}

// buildArchiveInfo resolves clouds and model types from the controller
// dump and cross-checks the model dump inventory against the model table.
func buildArchiveInfo(meta *corebackups.Metadata, payload *controllerDumpPayload, modelDumps map[string][]byte) (*domainrecovery.ArchiveInfo, error) {
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
	if payload.Controller[0].CACert == nil || payload.Controller[0].CAPrivateKey == nil {
		return nil, errors.Errorf("%s records no controller CA material", controllerDumpPath)
	}
	info.CACert = *payload.Controller[0].CACert
	info.CAPrivateKey = *payload.Controller[0].CAPrivateKey
	if payload.Controller[0].ModelUUID != "" &&
		info.ControllerModelUUID != payload.Controller[0].ModelUUID {
		return nil, errors.Errorf(
			"controller model mismatch: metadata records %q, controller row records %q",
			info.ControllerModelUUID, payload.Controller[0].ModelUUID)
	}
	// The manifest and the dump must agree on the controller's identity:
	// the recovered controller adopts the manifest's uuid, so a divergent
	// dump row means the archive is internally inconsistent.
	if payload.Controller[0].UUID != "" && payload.Controller[0].UUID != info.ControllerUUID {
		return nil, errors.Errorf(
			"controller uuid mismatch: metadata records %q, controller row records %q",
			info.ControllerUUID, payload.Controller[0].UUID)
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
		found := false
		for _, mi := range info.Models {
			if mi.UUID == uuid {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("archive contains a database dump for unknown model %q", uuid)
		}
	}

	// Fill the CAAS workload inventory from the model dumps: it powers
	// the read-only substrate check, which reports missing workload
	// objects instead of silently recreating them at the first
	// reconcile. The controller model is skipped: its namespace is
	// disposable bootstrap output, not surviving substrate.
	for i := range info.Models {
		mi := &info.Models[i]
		if mi.ModelType != "caas" || mi.UUID == info.ControllerModelUUID {
			continue
		}
		apps, err := inventoryFromModelDump(modelDumps[mi.UUID])
		if err != nil {
			return nil, errors.Errorf("model %q: %w", mi.Name, err)
		}
		mi.Applications = apps
	}
	return info, nil
}

// modelDumpInventory mirrors the model-dump tables the substrate
// inventory reads. Unknown tables and fields are ignored.
type modelDumpInventory struct {
	Application []struct {
		UUID   string `yaml:"uuid"`
		Name   string `yaml:"name"`
		LifeID int64  `yaml:"life_id"`
	} `yaml:"application"`
	Unit []struct {
		Name            string `yaml:"name"`
		LifeID          int64  `yaml:"life_id"`
		ApplicationUUID string `yaml:"application_uuid"`
		NetNodeUUID     string `yaml:"net_node_uuid"`
	} `yaml:"unit"`
	StorageFilesystem []struct {
		UUID       string `yaml:"uuid"`
		ProviderID string `yaml:"provider_id"`
		LifeID     int64  `yaml:"life_id"`
	} `yaml:"storage_filesystem"`
	StorageFilesystemAttachment []struct {
		StorageFilesystemUUID string `yaml:"storage_filesystem_uuid"`
		NetNodeUUID           string `yaml:"net_node_uuid"`
		LifeID                int64  `yaml:"life_id"`
	} `yaml:"storage_filesystem_attachment"`
}

// inventoryFromModelDump extracts the surviving-substrate inventory of
// one CAAS model: its alive applications, their alive units, and the
// persistent volume claim names recorded by alive storage filesystems.
// A claim name is attributed to the application whose unit attaches it.
func inventoryFromModelDump(data []byte) ([]domainrecovery.ApplicationInfo, error) {
	var envelope struct {
		Payload modelDumpInventory `yaml:"payload"`
	}
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return nil, errors.Errorf("decoding dump: %w", err)
	}
	inv := envelope.Payload

	// unitAppByNode maps a unit's net node to the unit's application,
	// resolving which application each attached volume belongs to.
	unitAppByNode := make(map[string]string, len(inv.Unit))
	for _, u := range inv.Unit {
		if u.LifeID == 2 || u.NetNodeUUID == "" {
			continue
		}
		unitAppByNode[u.NetNodeUUID] = u.ApplicationUUID
	}
	// provider_id is the persistent volume claim name: its unique index
	// is how the running controller re-attaches volumes it finds in the
	// cluster, so it is the name the substrate check must verify.
	filesystemClaim := make(map[string]string, len(inv.StorageFilesystem))
	for _, fs := range inv.StorageFilesystem {
		if fs.ProviderID == "" || fs.LifeID == 2 {
			continue
		}
		filesystemClaim[fs.UUID] = fs.ProviderID
	}
	claimsByApp := make(map[string][]string)
	for _, att := range inv.StorageFilesystemAttachment {
		if att.LifeID == 2 {
			continue
		}
		claim, ok := filesystemClaim[att.StorageFilesystemUUID]
		if !ok {
			continue
		}
		if app, ok := unitAppByNode[att.NetNodeUUID]; ok && app != "" {
			claimsByApp[app] = append(claimsByApp[app], claim)
		}
	}

	inventory := make([]domainrecovery.ApplicationInfo, 0, len(inv.Application))
	for _, app := range inv.Application {
		// Dead applications are recorded in the recovery summary as
		// pending removals; their substrate is not checked.
		if app.UUID == "" || app.Name == "" || app.LifeID == 2 {
			continue
		}
		info := domainrecovery.ApplicationInfo{
			UUID:                  app.UUID,
			Name:                  app.Name,
			FilesystemProviderIDs: claimsByApp[app.UUID],
		}
		for _, u := range inv.Unit {
			if u.ApplicationUUID == app.UUID && u.LifeID != 2 {
				info.Units = append(info.Units, u.Name)
			}
		}
		inventory = append(inventory, info)
	}
	return inventory, nil
}
