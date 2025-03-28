// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"encoding/json"

	"github.com/juju/description/v9"

	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

func (e *exportOperation) exportCharm(ctx context.Context, app description.Application, charm internalcharm.Charm) error {
	var lxdProfile string
	if profiler, ok := charm.(internalcharm.LXDProfiler); ok {
		var err error
		if lxdProfile, err = e.exportLXDProfile(profiler.LXDProfile()); err != nil {
			return errors.Errorf("cannot export LXD profile: %v", err)
		}
	}

	metadata, err := e.exportCharmMetadata(charm.Meta(), lxdProfile)
	if err != nil {
		return errors.Errorf("cannot export charm metadata: %v", err)
	}

	manifest, err := e.exportCharmManifest(charm.Manifest())
	if err != nil {
		return errors.Errorf("cannot export charm manifest: %v", err)
	}

	config, err := e.exportCharmConfig(charm.Config())
	if err != nil {
		return errors.Errorf("cannot export charm config: %v", err)
	}

	actions, err := e.exportCharmActions(charm.Actions())
	if err != nil {
		return errors.Errorf("cannot export charm actions: %v", err)
	}

	app.SetCharmMetadata(metadata)
	app.SetCharmManifest(manifest)
	app.SetCharmConfigs(config)
	app.SetCharmActions(actions)

	return nil
}

func (e *exportOperation) exportCharmMetadata(metadata *internalcharm.Meta, lxdProfile string) (description.CharmMetadataArgs, error) {
	// Assumes is a recursive structure, so we need to marshal it to JSON as
	// a string, to prevent YAML from trying to interpret it.
	var assumesBytes []byte
	if expr := metadata.Assumes; expr != nil {
		var err error
		assumesBytes, err = json.Marshal(expr)
		if err != nil {
			return description.CharmMetadataArgs{}, errors.Errorf("cannot marshal assumes: %v", err)
		}
	}

	runAs, err := exportCharmUser(metadata.CharmUser)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	provides, err := exportRelations(metadata.Provides)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	requires, err := exportRelations(metadata.Requires)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	peers, err := exportRelations(metadata.Peers)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	extraBindings := exportExtraBindings(metadata.ExtraBindings)

	storage, err := exportStorage(metadata.Storage)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	devices, err := exportDevices(metadata.Devices)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	containers, err := exportContainers(metadata.Containers)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	resources, err := exportResources(metadata.Resources)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	return description.CharmMetadataArgs{
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		Categories:     metadata.Categories,
		Tags:           metadata.Tags,
		Terms:          metadata.Terms,
		RunAs:          runAs,
		Assumes:        string(assumesBytes),
		MinJujuVersion: metadata.MinJujuVersion.String(),
		Provides:       provides,
		Requires:       requires,
		Peers:          peers,
		ExtraBindings:  extraBindings,
		Storage:        storage,
		Devices:        devices,
		Containers:     containers,
		Resources:      resources,
		LXDProfile:     lxdProfile,
	}, nil
}

func (e *exportOperation) exportLXDProfile(profile *internalcharm.LXDProfile) (string, error) {
	if profile == nil {
		return "", nil
	}

	// The LXD profile is encoded in the description package as a JSON blob.
	// This ensures consistency and prevents accidental encoding issues with
	// YAML.
	data, err := json.Marshal(profile)
	if err != nil {
		return "", errors.Capture(err)
	}

	return string(data), nil
}

func (e *exportOperation) exportCharmManifest(manifest *internalcharm.Manifest) (description.CharmManifestArgs, error) {
	if manifest == nil {
		return description.CharmManifestArgs{}, nil
	}

	bases, err := exportManifestBases(manifest.Bases)
	if err != nil {
		return description.CharmManifestArgs{}, errors.Capture(err)
	}

	return description.CharmManifestArgs{
		Bases: bases,
	}, nil
}

func (e *exportOperation) exportCharmConfig(config *internalcharm.Config) (description.CharmConfigsArgs, error) {
	if config == nil {
		return description.CharmConfigsArgs{}, nil
	}

	configs := make(map[string]description.CharmConfig, len(config.Options))
	for name, option := range config.Options {
		configs[name] = configType{
			typ:          option.Type,
			description:  option.Description,
			defaultValue: option.Default,
		}
	}

	return description.CharmConfigsArgs{
		Configs: configs,
	}, nil
}

func (e *exportOperation) exportCharmActions(actions *internalcharm.Actions) (description.CharmActionsArgs, error) {
	if actions == nil {
		return description.CharmActionsArgs{}, nil
	}

	result := make(map[string]description.CharmAction, len(actions.ActionSpecs))
	for name, action := range actions.ActionSpecs {
		result[name] = actionType{
			description:    action.Description,
			parallel:       action.Parallel,
			executionGroup: action.ExecutionGroup,
			parameters:     action.Params,
		}
	}

	return description.CharmActionsArgs{
		Actions: result,
	}, nil
}
