// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network/ipfamily"
	corestorage "github.com/juju/juju/core/storage"
	domainconstraints "github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/export/types/latest"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// agentStreamConfigKey is the model config key the model config view derives
// from the model's agent version stream.
const agentStreamConfigKey = "agent-stream"

// ControllerPreflightDump holds the subset of tables from the controller
// database dump that preflight validation inspects. Unknown tables are ignored;
// full decode happens agent-side during the load stage.
type ControllerPreflightDump struct {
	// Controller holds the controller table rows.
	Controller []latest.Controller `yaml:"controller"`
	// ControllerSSHServerPort holds the controller SSH server port rows.
	ControllerSSHServerPort []latest.ControllerSshServerPort `yaml:"controller_ssh_server_port"`
	// Cloud holds the cloud table rows.
	Cloud []latest.Cloud `yaml:"cloud"`
	// CloudType holds the cloud_type lookup rows.
	CloudType []latest.CloudType `yaml:"cloud_type"`
	// CloudRegion holds the cloud_region lookup rows.
	CloudRegion []latest.CloudRegion `yaml:"cloud_region"`
	// Model holds the model table rows.
	Model []latest.Model `yaml:"model"`
	// ModelType holds the model_type lookup rows.
	ModelType []latest.ModelType `yaml:"model_type"`
	// ControllerConfig holds the controller_config rows.
	ControllerConfig []latest.ControllerConfig `yaml:"controller_config"`
}

// DecodeControllerPreflight unmarshals the controller database dump YAML into
// the subset of tables preflight validation inspects. The envelope version is
// intentionally not parsed here: the agent-version gate implies matching dump
// formats, and the loader validates them.
func DecodeControllerPreflight(data []byte) (*ControllerPreflightDump, error) {
	var env struct {
		Payload ControllerPreflightDump `yaml:"payload"`
	}
	if err := yaml.Unmarshal(data, &env); err != nil {
		return nil, errors.Capture(err)
	}
	return &env.Payload, nil
}

// ControllerModelProvisioning holds the controller model provisioning inputs
// extracted from its database dump.
type ControllerModelProvisioning struct {
	// Config holds the archived model_config rows plus the derived
	// agent-stream value.
	Config map[string]string
	// ModelConstraints holds the archived controller model constraints.
	ModelConstraints constraints.Value
	// ControllerConstraints holds the archived controller application
	// constraints.
	ControllerConstraints constraints.Value
	// StoragePools holds the archived controller model storage pools in
	// the bootstrap representation.
	StoragePools map[string]storage.Attrs
}

// controllerModelDumpPayload selects the model-dump tables the controller
// model provisioning inputs read. Unknown tables are ignored.
type controllerModelDumpPayload struct {
	ModelConfig           []latest.ModelConfig           `yaml:"model_config"`
	AgentStream           []latest.AgentStream           `yaml:"agent_stream"`
	AgentVersion          []latest.AgentVersion          `yaml:"agent_version"`
	Constraint            []latest.Constraint            `yaml:"constraint"`
	ContainerType         []latest.ContainerType         `yaml:"container_type"`
	ConstraintTag         []latest.ConstraintTag         `yaml:"constraint_tag"`
	ConstraintSpace       []latest.ConstraintSpace       `yaml:"constraint_space"`
	ConstraintZone        []latest.ConstraintZone        `yaml:"constraint_zone"`
	ModelConstraint       []latest.ModelConstraint       `yaml:"model_constraint"`
	ApplicationController []latest.ApplicationController `yaml:"application_controller"`
	ApplicationConstraint []latest.ApplicationConstraint `yaml:"application_constraint"`
	StoragePool           []latest.StoragePool           `yaml:"storage_pool"`
	StoragePoolAttribute  []latest.StoragePoolAttribute  `yaml:"storage_pool_attribute"`
}

// DecodeControllerModelProvisioning reads the controller model provisioning
// inputs from a model database dump stream: the model config, the model and
// controller application constraints, and the storage pools.
func DecodeControllerModelProvisioning(r io.Reader) (ControllerModelProvisioning, error) {
	var envelope struct {
		Payload controllerModelDumpPayload `yaml:"payload"`
	}
	if err := yaml.NewDecoder(r).Decode(&envelope); err != nil {
		return ControllerModelProvisioning{}, errors.Errorf("decoding dump: %w", err)
	}
	payload := &envelope.Payload

	modelConfig := make(map[string]string, len(payload.ModelConfig))
	for _, mc := range payload.ModelConfig {
		modelConfig[mc.Key] = mc.Value
	}
	// agent-stream is a derived column of the model config view, resolved
	// through the model's agent version row.
	streamNames := make(map[int64]string, len(payload.AgentStream))
	for _, stream := range payload.AgentStream {
		if stream.ID != nil {
			streamNames[*stream.ID] = stream.Name
		}
	}
	for _, av := range payload.AgentVersion {
		if name := streamNames[av.StreamID]; name != "" {
			modelConfig[agentStreamConfigKey] = name
			break
		}
	}

	modelConstraints, err := modelConstraintFromDump(payload)
	if err != nil {
		return ControllerModelProvisioning{}, errors.Errorf("model constraints: %w", err)
	}
	controllerConstraints, err := controllerConstraintFromDump(payload)
	if err != nil {
		return ControllerModelProvisioning{}, errors.Errorf("controller constraints: %w", err)
	}
	return ControllerModelProvisioning{
		Config:                modelConfig,
		ModelConstraints:      modelConstraints,
		ControllerConstraints: controllerConstraints,
		StoragePools:          storagePoolsFromDump(payload),
	}, nil
}

// modelConstraintFromDump returns the archived model constraints. A model
// dump holds at most one model_constraint row: the model uuid is its primary
// key.
func modelConstraintFromDump(payload *controllerModelDumpPayload) (constraints.Value, error) {
	if len(payload.ModelConstraint) == 0 {
		return constraints.Value{}, nil
	}
	return decodeConstraint(payload.ModelConstraint[0].ConstraintUUID, payload)
}

// controllerConstraintFromDump returns the archived constraints of the
// controller application. An absent row, or one without constraints, means
// the controller was bootstrapped without any.
func controllerConstraintFromDump(payload *controllerModelDumpPayload) (constraints.Value, error) {
	if len(payload.ApplicationController) == 0 {
		return constraints.Value{}, nil
	}
	controllerApp := payload.ApplicationController[0].ApplicationUUID
	for _, ac := range payload.ApplicationConstraint {
		if ac.ApplicationUUID != controllerApp {
			continue
		}
		if ac.ConstraintUUID == nil || *ac.ConstraintUUID == "" {
			return constraints.Value{}, nil
		}
		return decodeConstraint(*ac.ConstraintUUID, payload)
	}
	return constraints.Value{}, nil
}

// storagePoolsFromDump returns the archived controller model storage pools in
// the bootstrap representation: each entry carries the pool name and type
// under their bootstrap keys alongside the pool's attributes.
func storagePoolsFromDump(payload *controllerModelDumpPayload) map[string]storage.Attrs {
	if len(payload.StoragePool) == 0 {
		return nil
	}
	pools := make(map[string]storage.Attrs, len(payload.StoragePool))
	namesByUUID := make(map[string]string, len(payload.StoragePool))
	for _, pool := range payload.StoragePool {
		namesByUUID[pool.UUID] = pool.Name
		pools[pool.Name] = storage.Attrs{
			corestorage.BootstrapStoragePoolNameKey: pool.Name,
			corestorage.BootstrapStoragePoolTypeKey: pool.Type,
		}
	}
	for _, attr := range payload.StoragePoolAttribute {
		attrs, ok := pools[namesByUUID[attr.StoragePoolUUID]]
		if !ok {
			continue
		}
		attrs[attr.Key] = attr.Value
	}
	return pools
}

// decodeConstraint converts the archived constraint row with the given uuid,
// including its tags, spaces, zones and container type, to a constraints
// value. An empty uuid means no constraints were recorded.
func decodeConstraint(uuid string, payload *controllerModelDumpPayload) (constraints.Value, error) {
	if uuid == "" {
		return constraints.Value{}, nil
	}
	var row *latest.Constraint
	for i := range payload.Constraint {
		if payload.Constraint[i].UUID == uuid {
			row = &payload.Constraint[i]
			break
		}
	}
	if row == nil {
		return constraints.Value{}, errors.Errorf("constraint %q not found", uuid)
	}

	cons := domainconstraints.Constraints{
		Arch:           row.Arch,
		RootDiskSource: row.RootDiskSource,
		InstanceRole:   row.InstanceRole,
		InstanceType:   row.InstanceType,
		VirtType:       row.VirtType,
		ImageID:        row.ImageID,
	}
	for _, num := range []struct {
		name  string
		value *int64
		set   func(uint64)
	}{
		{name: "cores", value: row.CpuCores, set: func(v uint64) { cons.CpuCores = &v }},
		{name: "cpu-power", value: row.CpuPower, set: func(v uint64) { cons.CpuPower = &v }},
		{name: "mem", value: row.Mem, set: func(v uint64) { cons.Mem = &v }},
		{name: "root-disk", value: row.RootDisk, set: func(v uint64) { cons.RootDisk = &v }},
	} {
		if num.value == nil {
			continue
		}
		// The column is nullable but unsigned in the domain: a negative
		// value is a corrupt dump, not a constraint to provision with.
		if *num.value < 0 {
			return constraints.Value{}, errors.Errorf(
				"constraint %q records a negative %s value %d", uuid, num.name, *num.value)
		}
		num.set(uint64(*num.value))
	}
	if row.AllocatePublicIp != nil {
		allocate := *row.AllocatePublicIp != 0
		cons.AllocatePublicIP = &allocate
	}
	if row.IpFamily != nil && *row.IpFamily != "" {
		family := ipfamily.IPFamily(*row.IpFamily)
		cons.IPFamily = &family
	}
	if row.ContainerTypeID != nil {
		for _, ct := range payload.ContainerType {
			if ct.ID == nil || *ct.ID != *row.ContainerTypeID {
				continue
			}
			containerType := instance.ContainerType(ct.Value)
			cons.Container = &containerType
			break
		}
	}

	tags := make([]string, 0, len(payload.ConstraintTag))
	for _, tag := range payload.ConstraintTag {
		if tag.ConstraintUUID == uuid {
			tags = append(tags, tag.Tag)
		}
	}
	if len(tags) != 0 {
		cons.Tags = &tags
	}
	spaces := make([]domainconstraints.SpaceConstraint, 0, len(payload.ConstraintSpace))
	for _, space := range payload.ConstraintSpace {
		if space.ConstraintUUID == uuid {
			spaces = append(spaces, domainconstraints.SpaceConstraint{
				SpaceName: space.Space,
				Exclude:   space.Exclude,
			})
		}
	}
	if len(spaces) != 0 {
		cons.Spaces = &spaces
	}
	zones := make([]string, 0, len(payload.ConstraintZone))
	for _, zone := range payload.ConstraintZone {
		if zone.ConstraintUUID == uuid {
			zones = append(zones, zone.Zone)
		}
	}
	if len(zones) != 0 {
		cons.Zones = &zones
	}
	return domainconstraints.EncodeConstraints(cons), nil
}

// modelDumpInventory selects the model-dump tables the substrate
// inventory reads. Unknown tables are ignored.
//
// Life filtering excludes only dead rows: dying entities still exist
// in the cluster, so they count as surviving substrate the preflight
// must account for.
type modelDumpInventory struct {
	Application                 []latest.Application                 `yaml:"application"`
	Unit                        []latest.Unit                        `yaml:"unit"`
	StorageFilesystem           []latest.StorageFilesystem           `yaml:"storage_filesystem"`
	StorageFilesystemAttachment []latest.StorageFilesystemAttachment `yaml:"storage_filesystem_attachment"`
}

// DecodeModelInventory extracts the surviving-substrate inventory of
// one Kubernetes model from its database dump stream: its alive applications,
// their alive units, and the persistent volume claim names recorded by alive
// storage filesystems. A claim name is attributed to the application whose
// unit attaches it.
func DecodeModelInventory(ctx context.Context, logger corelogger.Logger, r io.Reader) ([]recovery.ApplicationInfo, error) {
	var envelope struct {
		Payload modelDumpInventory `yaml:"payload"`
	}
	if err := yaml.NewDecoder(r).Decode(&envelope); err != nil {
		return nil, errors.Errorf("decoding dump: %w", err)
	}
	inv := envelope.Payload

	// unitAppByNode maps a unit's net node to the unit's application,
	// resolving which application each attached volume belongs to.
	unitAppByNode := make(map[string]string, len(inv.Unit))
	for _, u := range inv.Unit {
		if domainlife.Life(u.LifeID) == domainlife.Dead {
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
		if fs.ProviderID == nil || *fs.ProviderID == "" || domainlife.Life(fs.LifeID) == domainlife.Dead {
			continue
		}
		filesystemClaim[fs.UUID] = *fs.ProviderID
	}
	claimsByApp := make(map[string][]string)
	for _, att := range inv.StorageFilesystemAttachment {
		if domainlife.Life(att.LifeID) == domainlife.Dead {
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

	inventory := make([]recovery.ApplicationInfo, 0, len(inv.Application))
	for _, app := range inv.Application {
		// Dead applications are recorded in the recovery summary as
		// pending removals; their substrate is not checked.
		if domainlife.Life(app.LifeID) == domainlife.Dead {
			continue
		}
		// An alive application without its identity is not a row the
		// dump format produces; the dump is corrupt.
		if app.UUID == "" || app.Name == "" {
			return nil, errors.Errorf("dump records an alive application with an empty uuid or name")
		}
		info := recovery.ApplicationInfo{
			UUID:                  app.UUID,
			Name:                  app.Name,
			FilesystemProviderIDs: claimsByApp[app.UUID],
		}
		for _, u := range inv.Unit {
			if u.ApplicationUUID == app.UUID && domainlife.Life(u.LifeID) != domainlife.Dead {
				info.Units = append(info.Units, u.Name)
			}
		}
		inventory = append(inventory, info)
	}
	return inventory, nil
}
