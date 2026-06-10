// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"sort"

	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v2"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/machine"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/rpc/params"
)

// assembledModel carries the v8 wire envelope for the migrating model plus the
// binary-transfer references the worker feeds to UploadBinaries. Both are
// produced by the same assembly pass so they cannot diverge.
type assembledModel struct {
	// envelope is the wire envelope sent to the target's v8 Prechecks and
	// Import methods.
	envelope params.SerializedModelV2

	// charms are the charm URLs to transfer via /migrate/charms.
	charms []string

	// tools are the agent binaries to transfer via /migrate/tools, keyed on
	// the SHA256 sum and referenced to a binary version.
	tools map[string]semversion.Binary

	// resources are the application resources to transfer via
	// /migrate/resources.
	resources []coreresource.Resource
}

// assembleEnvelope builds a fresh params.SerializedModelV2 envelope for this
// model from the local domain services: the model-DB export payload, the
// controller-DB semantic facts, and the charm/tools/resources binary
// references. The payload size is validated against the serialized-model
// payload limit before the envelope is handed to any RPC.
func (w *Worker) assembleEnvelope(ctx context.Context, migrationUUID string) (assembledModel, error) {
	var empty assembledModel

	export, err := w.config.ExportService.Export(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "exporting model")
	}
	// Marshal only the concrete generated model-DB payload. The
	// domain/export.ModelExport wrapper is never serialized;
	// envelope.PayloadVersion is the single wire version authority.
	payload, err := goyaml.Marshal(export.Payload)
	if err != nil {
		return empty, errors.Annotate(err, "marshalling model payload")
	}

	info, err := w.config.ModelMigrationService.GetControllerModelInfo(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "reading controller facts for model")
	}
	envelope := envelopeFromControllerModelInfo(info, migrationUUID)
	envelope.PayloadVersion = export.Version
	envelope.Payload = payload

	locators, err := w.config.CharmService.ListCharmLocators(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "listing model charms")
	}
	charms, err := charmURLsFromLocators(locators)
	if err != nil {
		return empty, errors.Trace(err)
	}
	envelope.Charms = charms

	machineTools, err := w.config.ModelAgentService.GetMachinesAgentBinaryMetadata(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "listing machine agent binaries")
	}
	unitTools, err := w.config.ModelAgentService.GetUnitsAgentBinaryMetadata(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "listing unit agent binaries")
	}
	tools, envelopeTools := toolsForEnvelope(machineTools, unitTools)
	envelope.Tools = envelopeTools

	exported, err := w.config.ResourceService.ListAllModelResources(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "listing model resources")
	}
	envelope.Resources = resourcesForEnvelope(exported)

	if err := validateEnvelopeSize(envelope); err != nil {
		return empty, errors.Trace(err)
	}
	return assembledModel{
		envelope:  envelope,
		charms:    charms,
		tools:     tools,
		resources: exported,
	}, nil
}

// validateEnvelopeSize rejects envelopes whose model-DB payload exceeds the
// serialized-model payload limit. The source validates locally before RPC so
// an oversized payload never travels to the target.
func validateEnvelopeSize(envelope params.SerializedModelV2) error {
	if size := len(envelope.Payload); size > params.SerializedModelV2PayloadLimit {
		return errors.Errorf(
			"model payload is %d bytes which exceeds the %d byte limit",
			size, params.SerializedModelV2PayloadLimit,
		)
	}
	return nil
}

// envelopeFromControllerModelInfo converts the controller-DB semantic facts
// for the migrating model into their wire envelope form. The migration UUID
// of the active source migration is recorded as the envelope's
// SourceMigrationUUID.
func envelopeFromControllerModelInfo(
	info modelmigration.ControllerModelInfo, migrationUUID string,
) params.SerializedModelV2 {
	envelope := params.SerializedModelV2{
		ModelInfo: params.SerializedModelInfo{
			UUID:                info.ModelInfo.UUID,
			Name:                info.ModelInfo.Name,
			Qualifier:           info.ModelInfo.Qualifier,
			Type:                info.ModelInfo.Type,
			Cloud:               info.ModelInfo.Cloud,
			CloudRegion:         info.ModelInfo.CloudRegion,
			CredentialName:      info.ModelInfo.CredentialName,
			CredentialOwner:     info.ModelInfo.CredentialOwner,
			Life:                info.ModelInfo.Life,
			SourceMigrationUUID: migrationUUID,
		},
	}
	for _, user := range info.Users {
		envelope.Users = append(envelope.Users, params.ModelUser{
			Name:        user.Name,
			DisplayName: user.DisplayName,
			CreatedBy:   user.CreatedBy,
			CreatedAt:   user.CreatedAt,
			Removed:     user.Removed,
			External:    user.External,
		})
		if user.LastLogin != nil {
			envelope.LastLogins = append(envelope.LastLogins, params.ModelLastLogin{
				Username: user.Name,
				Time:     *user.LastLogin,
			})
		}
	}
	if cred := info.ModelCredential; cred != nil {
		envelope.ModelCredential = &params.ModelCloudCredential{
			Cloud:         cred.Cloud,
			Owner:         cred.Owner,
			Name:          cred.Name,
			AuthType:      cred.AuthType,
			Attributes:    cred.Attributes,
			Revoked:       cred.Revoked,
			Invalid:       cred.Invalid,
			InvalidReason: cred.InvalidReason,
		}
	}
	for _, perm := range info.Permissions {
		envelope.Permissions = append(envelope.Permissions, params.ModelPermission{
			ObjectType:  perm.ObjectType,
			GrantOn:     perm.GrantOn,
			SubjectName: perm.SubjectName,
			Access:      perm.Access,
		})
	}
	for _, key := range info.AuthorizedKeys {
		envelope.AuthorizedKeys = append(envelope.AuthorizedKeys, params.ModelAuthorizedKey{
			Username:  key.Username,
			PublicKey: key.PublicKey,
		})
	}
	if backend := info.SecretBackend; backend != nil {
		envelope.SecretBackend = &params.ModelSecretBackend{
			Name:        backend.Name,
			BackendType: backend.BackendType,
		}
	}
	for _, ref := range info.SecretBackendRefs {
		envelope.SecretBackendRefs = append(envelope.SecretBackendRefs, params.SecretBackendReference{
			BackendName:        ref.BackendName,
			SecretRevisionUUID: ref.SecretRevisionUUID,
			SecretID:           ref.SecretID,
		})
	}
	for _, leader := range info.Leaders {
		envelope.Leases = append(envelope.Leases, params.Lease{
			Type:   corelease.ApplicationLeadershipNamespace,
			Name:   leader.Application,
			Holder: leader.Leader,
		})
	}
	for _, metadata := range info.CloudImageMetadata {
		envelope.CloudImageMetadata = append(envelope.CloudImageMetadata, params.ModelCloudImageMetadata{
			Stream:          metadata.Stream,
			Region:          metadata.Region,
			Version:         metadata.Version,
			Arch:            metadata.Arch,
			VirtType:        metadata.VirtType,
			RootStorageType: metadata.RootStorageType,
			RootStorageSize: metadata.RootStorageSize,
			Source:          metadata.Source,
			Priority:        metadata.Priority,
			ImageId:         metadata.ImageID,
			CreatedAt:       metadata.CreatedAt,
		})
	}
	for _, controller := range info.ExternalControllers {
		envelope.ExternalControllers = append(envelope.ExternalControllers, params.ExternalControllerRef{
			UUID:           controller.UUID,
			Alias:          controller.Alias,
			CACert:         controller.CACert,
			Addresses:      controller.Addresses,
			ConsumedModels: controller.ConsumedModels,
		})
	}
	return envelope
}

// charmURLsFromLocators converts charm locators into the charm URL strings
// used by the wire envelope and the charm binary uploads.
func charmURLsFromLocators(locators []applicationcharm.CharmLocator) ([]string, error) {
	if len(locators) == 0 {
		return nil, nil
	}
	urls := make([]string, 0, len(locators))
	for _, locator := range locators {
		url, err := charmURLFromLocator(locator)
		if err != nil {
			return nil, errors.Trace(err)
		}
		urls = append(urls, url)
	}
	return urls, nil
}

func charmURLFromLocator(locator applicationcharm.CharmLocator) (string, error) {
	schema, err := charmSchemaFromSource(locator.Source)
	if err != nil {
		return "", errors.Trace(err)
	}
	arch, err := archFromArchitecture(locator.Architecture)
	if err != nil {
		return "", errors.Trace(err)
	}
	url := deploymentcharm.URL{
		Schema:       schema,
		Name:         locator.Name,
		Revision:     locator.Revision,
		Architecture: arch,
	}
	return url.String(), nil
}

func charmSchemaFromSource(source applicationcharm.CharmSource) (string, error) {
	switch source {
	case applicationcharm.CharmHubSource:
		return deploymentcharm.CharmHub.String(), nil
	case applicationcharm.LocalSource:
		return deploymentcharm.Local.String(), nil
	default:
		return "", errors.Errorf("unsupported charm source %q", source)
	}
}

func archFromArchitecture(a architecture.Architecture) (string, error) {
	switch a {
	case architecture.AMD64:
		return "amd64", nil
	case architecture.ARM64:
		return "arm64", nil
	case architecture.PPC64EL:
		return "ppc64el", nil
	case architecture.S390X:
		return "s390x", nil
	case architecture.RISCV64:
		return "riscv64", nil
	case architecture.Unknown:
		// A charm uploaded without an architecture has none in its URL.
		return "", nil
	default:
		return "", errors.Errorf("unsupported architecture %d", a)
	}
}

// toolsForEnvelope merges the agent binaries reported for machines and units,
// deduplicated by SHA256, into the upload map keyed on SHA256 and the wire
// envelope references. coreagentbinary.Version carries no OS release; 4.x
// agents are ubuntu-only so the binary version release is always "ubuntu".
func toolsForEnvelope(
	machineTools map[machine.Name]coreagentbinary.Metadata,
	unitTools map[unit.Name]coreagentbinary.Metadata,
) (map[string]semversion.Binary, []params.SerializedModelTools) {
	tools := make(map[string]semversion.Binary)
	addTool := func(metadata coreagentbinary.Metadata) {
		tools[metadata.SHA256] = semversion.Binary{
			Number:  metadata.Version.Number,
			Release: "ubuntu",
			Arch:    metadata.Version.Arch,
		}
	}
	for _, metadata := range machineTools {
		addTool(metadata)
	}
	for _, metadata := range unitTools {
		addTool(metadata)
	}
	if len(tools) == 0 {
		return tools, nil
	}

	envelopeTools := make([]params.SerializedModelTools, 0, len(tools))
	for sha256, version := range tools {
		envelopeTools = append(envelopeTools, params.SerializedModelTools{
			Version: version.String(),
			URI:     "/tools/" + version.String(),
			SHA256:  sha256,
		})
	}
	sort.Slice(envelopeTools, func(i, j int) bool {
		if envelopeTools[i].Version != envelopeTools[j].Version {
			return envelopeTools[i].Version < envelopeTools[j].Version
		}
		return envelopeTools[i].SHA256 < envelopeTools[j].SHA256
	})
	return tools, envelopeTools
}

// resourcesForEnvelope converts the application resources to their wire
// envelope references. Unit resource rows travel inside the model-DB payload,
// so the envelope carries application-level resources only, matching the
// legacy serialized model.
func resourcesForEnvelope(resources []coreresource.Resource) []params.SerializedModelResource {
	if len(resources) == 0 {
		return nil
	}
	out := make([]params.SerializedModelResource, 0, len(resources))
	for _, res := range resources {
		out = append(out, params.SerializedModelResource{
			Application:    res.ApplicationName,
			Name:           res.Name,
			Revision:       res.Revision,
			Type:           res.Type.String(),
			Origin:         res.Origin.String(),
			FingerprintHex: res.Fingerprint.Hex(),
			Size:           res.Size,
			Timestamp:      res.Timestamp,
			Username:       res.RetrievedBy,
		})
	}
	return out
}
