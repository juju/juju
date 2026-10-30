// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	"github.com/juju/juju/domain/export/types/latest"
	v4_0_12 "github.com/juju/juju/domain/export/types/v4_0_12"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// PayloadDecodeFunc decodes YAML bytes into the concrete generated
// model- or controller-export payload type (vX_Y_Z.ModelExport or
// vX_Y_Z.ControllerExport) for one export version. Both registries below are
// keyed on it; which payload kind comes back is fixed by the registry the
// decoder was looked up in.
type PayloadDecodeFunc func(data []byte) (any, error)

// payloadDecoders maps each supported export version to the decoder producing
// its concrete generated payload type. Every entry of [ExportVersions] must
// have a decoder here; the completeness is asserted by tests so that adding a
// new export version forces this registry to be updated. [ProjectionViewForPayload]
// does not need a per-version entry: it runs on the transformed latest payload.
var payloadDecoders = map[semversion.Number]PayloadDecodeFunc{
	semversion.MustParse("4.0.12"): decodePayload[v4_0_12.ModelExport],
	semversion.MustParse("4.1.0"):  decodeLookupPayload[v4_1_0.ModelExport](modelLookupIDTablesV410),
}

// These are the single-column INT lookup keys made NOT NULL in 4.1.0.
// Model changestream lookups are target-local and not consumed by import.
// Keep the lists version-specific: other scalar keys retain their wire contract.
const modelLookupIDTablesV410 = `
agent_stream
architecture
block_command_type
block_device_provenance
charm_config_type
charm_provenance
charm_relation_role
charm_relation_scope
charm_resource_kind
charm_run_as_kind
charm_source
charm_storage_kind
container_type
hash_kind
ip_address_config_type
ip_address_origin
ip_address_scope
ip_address_type
k8s_pod_status_value
life
link_layer_device_type
machine_cloud_instance_status_value
machine_placement_scope
machine_status_value
network_address_scope
operation_task_status_value
os
password_hash_algorithm
protocol
resolve_mode
resource_origin_type
resource_retrieved_by_type
resource_state
secret_grant_scope_type
secret_grant_subject_type
secret_role
secret_rotate_policy
ssh_key_algorithm_type
storage_filesystem_status_value
storage_kind
storage_provision_scope
storage_volume_device_type
storage_volume_status_value
unit_agent_status_value
virtual_port_type
workload_status_value
`

const controllerLookupIDTablesV410 = `
architecture
auth_type
change_log_edit_type
change_log_namespace
cloud_type
lease_type
life
model_type
object_store_drain_phase_type
password_hash_algorithm
permission_access_type
permission_object_access
permission_object_type
secret_backend_type
ssh_fingerprint_hash_algorithm
ssh_key_algorithm_type
upgrade_state_type
`

// decodeLookupPayload validates raw lookup IDs before scalar decoding turns
// omitted or NULL IDs into zero. Decoding through yaml.Node preserves YAML
// aliases and merge keys in both validation and the concrete payload.
func decodeLookupPayload[T any](lookupTables string) PayloadDecodeFunc {
	return func(data []byte) (any, error) {
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil, errors.Capture(err)
		}
		var tables map[string]yaml.Node
		if err := document.Decode(&tables); err != nil {
			return nil, errors.Capture(err)
		}
		for table := range strings.FieldsSeq(lookupTables) {
			node, ok := tables[table]
			if !ok {
				continue
			}
			var rows []*struct {
				ID *int64 `yaml:"id"`
			}
			if err := node.Decode(&rows); err != nil {
				return nil, errors.Errorf("decoding table %q: %w", table, err)
			}
			for i, row := range rows {
				if row == nil || row.ID == nil {
					return nil, errors.Errorf("table %q row %d: id must be present and non-NULL", table, i)
				}
			}
		}
		var out T
		if err := document.Decode(&out); err != nil {
			return nil, errors.Capture(err)
		}
		return out, nil
	}
}

func decodePayload[T any](data []byte) (any, error) {
	out, err := decodeInto[T](data)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return out, nil
}

// decodeInto decodes YAML data into a concrete generated payload value. The
// decoder uses gopkg.in/yaml.v3; the generated payload structs carry yaml
// struct tags, so the source-side encoder must marshal with matching tag
// semantics.
func decodeInto[T any](data []byte) (T, error) {
	var out T
	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, errors.Capture(err)
	}
	return out, nil
}

// DecodePayload decodes YAML data into the concrete generated
// vX_Y_Z.ModelExport payload type for the given export version. It returns an
// error satisfying [coreerrors.NotSupported] when the version is not a known
// export payload version, and an error satisfying [coreerrors.NotValid] when
// the bytes cannot be decoded as that version's payload. In 4.1.0, consumed
// lookup rows must carry non-NULL IDs; zero is a valid ID. Target-local model
// changestream lookups are not validated.
//
// An unknown version is reported through [CheckPayloadVersionSupported] so the
// operator is told which controller to upgrade rather than only which version
// was rejected.
func DecodePayload(version semversion.Number, data []byte) (any, error) {
	decode, ok := payloadDecoders[version]
	if !ok {
		if err := CheckPayloadVersionSupported(version); err != nil {
			return nil, errors.Capture(err)
		}
		// No decoder found for the version.
		return nil, errors.Errorf(
			"model export payload version %q has no decoder: %w", version, coreerrors.NotSupported)
	}
	payload, err := decode(data)
	if err != nil {
		return nil, errors.Errorf(
			"decoding model export payload at version %q: %w", version, err,
		).Add(coreerrors.NotValid)
	}
	return payload, nil
}

// controllerPayloadDecoders maps each supported controller export version to
// the decoder producing its concrete generated payload type. Every entry of
// [ControllerExportVersions] must have a decoder here; completeness is
// asserted by tests.
var controllerPayloadDecoders = map[semversion.Number]PayloadDecodeFunc{
	semversion.MustParse("4.1.0"): decodeLookupPayload[ctrlv4_1_0.ControllerExport](controllerLookupIDTablesV410),
}

// DecodeControllerPayload decodes YAML into the concrete controller payload
// type for the given version. Errors mirror DecodePayload semantics:
// [coreerrors.NotSupported] for an unknown version and [coreerrors.NotValid]
// for undecodable bytes or 4.1.0 lookup rows without non-NULL IDs. Zero is a
// valid lookup ID.
func DecodeControllerPayload(version semversion.Number, data []byte) (any, error) {
	decode, ok := controllerPayloadDecoders[version]
	if !ok {
		if err := checkControllerPayloadVersionSupported(version); err != nil {
			return nil, errors.Capture(err)
		}
		// No decoder found for the version.
		return nil, errors.Errorf(
			"controller export payload version %q has no decoder: %w", version, coreerrors.NotSupported)
	}
	payload, err := decode(data)
	if err != nil {
		return nil, errors.Errorf(
			"decoding controller export payload at version %q: %w", version, err,
		).Add(coreerrors.NotValid)
	}
	return payload, nil
}

// checkControllerPayloadVersionSupported reports whether this controller
// holds the given controller-export schema version, returning an error
// satisfying [coreerrors.NotSupported] when it does not. The message names
// the controller to upgrade, mirroring [CheckPayloadVersionSupported] but
// evaluated against [ControllerExportVersions].
//
// Unlike [CheckPayloadVersionSupported], which the migrationtarget facade calls
// to precheck a version before the payload is fetched, this stays unexported:
// the controller payload is only ever decoded, by [DecodeControllerPayload]
// right below, so there is nothing to precheck separately. Export it when the
// backups import path needs to validate a version on its own.
func checkControllerPayloadVersionSupported(version semversion.Number) error {
	if slices.Contains(ControllerExportVersions, version) {
		return nil
	}
	return errors.Errorf(
		"controller export payload version %q is not one of the controller export formats this controller supports (%v); upgrade this controller first: %w",
		version, ControllerExportVersions, coreerrors.NotSupported)
}

// agentStreamConfigKey is the model_config row key holding the model's
// configured agent stream. It mirrors environs/config.AgentStreamKey; the
// constant is duplicated rather than imported so this package, which only
// decodes and projects payload data, does not take on the much larger
// environs/config dependency graph for a single well-known string.
const agentStreamConfigKey = "agent-stream"

// ProjectionView is the version-neutral projection of decoded model export
// payload fields needed by migration prechecks and target-side bootstrap.
type ProjectionView struct {
	// AgentTargetVersion is the model's target agent version from the
	// payload's agent_version row. It is zero when the payload carries no
	// agent_version row.
	AgentTargetVersion semversion.Number

	// AgentStream is the model's configured agent stream, read from the
	// payload's model_config row keyed "agent-stream". It is empty when the
	// payload carries no such row, meaning the source used the default
	// stream.
	AgentStream string
}

// ProjectionViewForPayload builds the precheck projection from the transformed,
// target-version model-DB payload. Because the transformer normalizes every
// source version to [latest.ModelExport] before this runs, the projection only
// ever handles one payload shape: adding a new export version needs a decoder
// entry and a transformer step, but no change here.
func ProjectionViewForPayload(payload latest.ModelExport) (ProjectionView, error) {
	var view ProjectionView
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return ProjectionView{}, errors.Capture(err)
	}
	setAgentStream(&view, len(payload.ModelConfig), func(i int) (string, string) {
		return payload.ModelConfig[i].Key, payload.ModelConfig[i].Value
	})
	return view, nil
}

// setAgentTargetVersion parses the payload's single agent_version row into the
// view. The agent_version table holds at most one row; more than one is a
// malformed payload.
func setAgentTargetVersion(view *ProjectionView, rows int, targetVersion func(int) string) error {
	if rows == 0 {
		return nil
	}
	if rows > 1 {
		return errors.Errorf(
			"model export payload has %d agent_version rows, expected at most 1 %w",
			rows, coreerrors.NotValid)
	}
	parsed, err := semversion.Parse(targetVersion(0))
	if err != nil {
		return errors.Errorf(
			"parsing model export payload agent target version: %w", err,
		).Add(coreerrors.NotValid)
	}
	view.AgentTargetVersion = parsed
	return nil
}

// setAgentStream scans the payload's model_config rows for the row keyed
// "agent-stream" into the view. A missing row is not an error: the source
// simply used the default stream.
func setAgentStream(view *ProjectionView, rows int, row func(i int) (key, value string)) {
	for i := range rows {
		key, value := row(i)
		if key == agentStreamConfigKey {
			view.AgentStream = value
			return
		}
	}
}
