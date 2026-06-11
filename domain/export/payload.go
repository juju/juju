// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"github.com/juju/collections/set"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	v4_0_4 "github.com/juju/juju/domain/export/types/v4_0_4"
	v4_0_6 "github.com/juju/juju/domain/export/types/v4_0_6"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// PayloadDecodeFunc decodes YAML bytes into the concrete generated
// *vX_Y_Z.ModelExport payload type for one export version.
type PayloadDecodeFunc func(data []byte) (any, error)

// payloadDecoders maps each supported export version to the decoder producing
// its concrete generated payload type. Every entry of [ExportVersions] must
// have a decoder here; the completeness is asserted by tests so that adding a
// new export version forces this registry (and [StaticCheckViewFor]) to be
// updated.
var payloadDecoders = map[semversion.Number]PayloadDecodeFunc{
	semversion.MustParse("4.0.4"): decodeInto[v4_0_4.ModelExport],
	semversion.MustParse("4.0.6"): decodeInto[v4_0_6.ModelExport],
	semversion.MustParse("4.1.0"): decodeInto[v4_1_0.ModelExport],
}

// decodeInto decodes YAML data into a freshly allocated *T. The decoder uses
// gopkg.in/yaml.v3; the generated payload structs carry yaml struct tags, so
// the source-side encoder must marshal with matching tag semantics.
func decodeInto[T any](data []byte) (any, error) {
	out := new(T)
	if err := yaml.Unmarshal(data, out); err != nil {
		return nil, errors.Capture(err)
	}
	return out, nil
}

// DecodePayload decodes YAML data into the concrete generated
// *vX_Y_Z.ModelExport payload type for the given export version. It returns an
// error satisfying [coreerrors.NotSupported] when the version is not a known
// export payload version, and an error satisfying [coreerrors.NotValid] when
// the bytes cannot be decoded as that version's payload.
func DecodePayload(version semversion.Number, data []byte) (any, error) {
	decode, ok := payloadDecoders[version]
	if !ok {
		return nil, errors.Errorf(
			"model export payload version %q %w", version, coreerrors.NotSupported)
	}
	payload, err := decode(data)
	if err != nil {
		return nil, errors.Errorf(
			"decoding model export payload at version %q: %w", version, err,
		).Add(coreerrors.NotValid)
	}
	return payload, nil
}

// StaticCheckView is the version-neutral projection of the decoded model
// export payload fields needed by static migration prechecks.
type StaticCheckView struct {
	// Applications maps application name to the UUID of the charm it uses.
	Applications map[string]string

	// CharmUUIDsWithManifestBases holds the UUIDs of charms that have at
	// least one charm_manifest_base row.
	CharmUUIDsWithManifestBases set.Strings

	// ModelConfig holds the model config key/value rows.
	ModelConfig map[string]any

	// AgentTargetVersion is the model's target agent version from the
	// payload's agent_version row. It is zero when the payload carries no
	// agent_version row.
	AgentTargetVersion semversion.Number
}

// StaticCheckViewFor builds the static-check projection from a payload value
// returned by [DecodePayload]. It returns an error satisfying
// [coreerrors.NotSupported] for payload types not known to this registry.
func StaticCheckViewFor(payload any) (StaticCheckView, error) {
	switch p := payload.(type) {
	case *v4_0_4.ModelExport:
		return buildStaticCheckViewV4_0_4(p)
	case *v4_0_6.ModelExport:
		return buildStaticCheckViewV4_0_6(p)
	case *v4_1_0.ModelExport:
		return buildStaticCheckViewV4_1_0(p)
	default:
		return StaticCheckView{}, errors.Errorf(
			"model export payload type %T %w", payload, coreerrors.NotSupported)
	}
}

func buildStaticCheckViewV4_0_4(payload *v4_0_4.ModelExport) (StaticCheckView, error) {
	view := StaticCheckView{
		Applications:                make(map[string]string, len(payload.Application)),
		CharmUUIDsWithManifestBases: set.NewStrings(),
		ModelConfig:                 make(map[string]any, len(payload.ModelConfig)),
	}
	for _, app := range payload.Application {
		view.Applications[app.Name] = app.CharmUUID
	}
	for _, base := range payload.CharmManifestBase {
		view.CharmUUIDsWithManifestBases.Add(base.CharmUUID)
	}
	for _, cfg := range payload.ModelConfig {
		view.ModelConfig[cfg.Key] = cfg.Value
	}
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return StaticCheckView{}, errors.Capture(err)
	}
	return view, nil
}

func buildStaticCheckViewV4_0_6(payload *v4_0_6.ModelExport) (StaticCheckView, error) {
	view := StaticCheckView{
		Applications:                make(map[string]string, len(payload.Application)),
		CharmUUIDsWithManifestBases: set.NewStrings(),
		ModelConfig:                 make(map[string]any, len(payload.ModelConfig)),
	}
	for _, app := range payload.Application {
		view.Applications[app.Name] = app.CharmUUID
	}
	for _, base := range payload.CharmManifestBase {
		view.CharmUUIDsWithManifestBases.Add(base.CharmUUID)
	}
	for _, cfg := range payload.ModelConfig {
		view.ModelConfig[cfg.Key] = cfg.Value
	}
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return StaticCheckView{}, errors.Capture(err)
	}
	return view, nil
}

func buildStaticCheckViewV4_1_0(payload *v4_1_0.ModelExport) (StaticCheckView, error) {
	view := StaticCheckView{
		Applications:                make(map[string]string, len(payload.Application)),
		CharmUUIDsWithManifestBases: set.NewStrings(),
		ModelConfig:                 make(map[string]any, len(payload.ModelConfig)),
	}
	for _, app := range payload.Application {
		view.Applications[app.Name] = app.CharmUUID
	}
	for _, base := range payload.CharmManifestBase {
		view.CharmUUIDsWithManifestBases.Add(base.CharmUUID)
	}
	for _, cfg := range payload.ModelConfig {
		view.ModelConfig[cfg.Key] = cfg.Value
	}
	if err := setAgentTargetVersion(&view, len(payload.AgentVersion), func(i int) string {
		return payload.AgentVersion[i].TargetVersion
	}); err != nil {
		return StaticCheckView{}, errors.Capture(err)
	}
	return view, nil
}

// setAgentTargetVersion parses the payload's single agent_version row into the
// view. The agent_version table holds at most one row; more than one is a
// malformed payload.
func setAgentTargetVersion(view *StaticCheckView, rows int, targetVersion func(int) string) error {
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
