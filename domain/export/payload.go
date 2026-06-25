// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	v4_0_11 "github.com/juju/juju/domain/export/types/v4_0_11"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// PayloadDecodeFunc decodes YAML bytes into the concrete generated
// vX_Y_Z.ModelExport payload type for one export version.
type PayloadDecodeFunc func(data []byte) (any, error)

// payloadDecoders maps each supported export version to the decoder producing
// its concrete generated payload type. Every entry of [ExportVersions] must
// have a decoder here; the completeness is asserted by tests so that adding a
// new export version forces this registry (and [ProjectionViewForPayload]) to
// be updated.
var payloadDecoders = map[semversion.Number]PayloadDecodeFunc{
	semversion.MustParse("4.0.11"): decodePayload[v4_0_11.ModelExport],
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

// ProjectionView is the version-neutral projection of decoded model export
// payload fields needed by migration prechecks.
type ProjectionView struct {
	// AgentTargetVersion is the model's target agent version from the
	// payload's agent_version row. It is zero when the payload carries no
	// agent_version row.
	AgentTargetVersion semversion.Number
}

// ProjectionViewForPayload builds the precheck projection from a payload value
// returned by [DecodePayload]. It returns an error satisfying
// [coreerrors.NotSupported] for payload types not known to this registry.
func ProjectionViewForPayload(payload any) (ProjectionView, error) {
	switch p := payload.(type) {
	case v4_0_11.ModelExport:
		return buildProjectionViewV4_0_11(p)
	case v4_1_0.ModelExport:
		return buildProjectionViewV4_1_0(p)
	default:
		return ProjectionView{}, errors.Errorf(
			"model export payload type %T %w", payload, coreerrors.NotSupported)
	}
}

func buildProjectionViewV4_0_11(payload v4_0_11.ModelExport) (ProjectionView, error) {
	var view ProjectionView
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return ProjectionView{}, errors.Capture(err)
	}
	return view, nil
}

func buildProjectionViewV4_1_0(payload v4_1_0.ModelExport) (ProjectionView, error) {
	var view ProjectionView
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return ProjectionView{}, errors.Capture(err)
	}
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
