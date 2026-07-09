// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export/types/latest"
	v4_0_12 "github.com/juju/juju/domain/export/types/v4_0_12"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// PayloadDecodeFunc decodes YAML bytes into the concrete generated
// vX_Y_Z.ModelExport payload type for one export version.
type PayloadDecodeFunc func(data []byte) (any, error)

// payloadDecoders maps each supported export version to the decoder producing
// its concrete generated payload type. Every entry of [ExportVersions] must
// have a decoder here; the completeness is asserted by tests so that adding a
// new export version forces this registry to be updated. [ProjectionViewForPayload]
// does not need a per-version entry: it runs on the transformed latest payload.
var payloadDecoders = map[semversion.Number]PayloadDecodeFunc{
	semversion.MustParse("4.0.12"): decodePayload[v4_0_12.ModelExport],
	semversion.MustParse("4.1.0"):  decodePayload[v4_1_0.ModelExport],
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
// the bytes cannot be decoded as that version's payload.
func DecodePayload(version semversion.Number, data []byte) (any, error) {
	decode, ok := payloadDecoders[version]
	if !ok {
		return nil, errors.Errorf(
			"model export payload version %q: %w", version, coreerrors.NotSupported)
	}
	payload, err := decode(data)
	if err != nil {
		return nil, errors.Errorf(
			"decoding model export payload at version %q: %w", version, err,
		).Add(coreerrors.NotValid)
	}
	return payload, nil
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
