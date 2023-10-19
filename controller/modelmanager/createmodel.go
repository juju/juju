// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"
	"github.com/juju/version/v2"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/tools"
)

var (
	logger = loggo.GetLogger("juju.controller.modelmanager")
)

// ModelConfigCreator provides a method of creating a new model config.
//
// The zero value of ModelConfigCreator is usable with the limitations
// noted on each struct field.
type ModelConfigCreator struct {
	// Provider will be used to obtain EnvironProviders for preparing
	// and validating configuration.
	Provider func(string) (environs.EnvironProvider, error)

	// FindTools, if non-nil, will be used to validate the agent-version
	// value in NewModelConfig if it differs from the base configuration.
	//
	// If FindTools is nil, agent-version may not be different to the
	// base configuration.
	FindTools func(version.Number) (tools.List, error)
}

// NewModelConfig returns a new model config given a base (controller) config
// and a set of attributes that will be specific to the new model, overriding
// any non-restricted attributes in the base configuration. The resulting
// config will be suitable for creating a new model in state.
//
// If "attrs" does not include a UUID, a new, random one will be generated
// and added to the config.
//
// The config will be validated with the provider before being returned.
func (c ModelConfigCreator) NewModelConfig(
	cloud environscloudspec.CloudSpec,
	base *config.Config,
	attrs map[string]interface{},
) (*config.Config, error) {

	if err := c.checkVersion(base, attrs); err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := c.Provider(cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Generate a new UUID for the model as necessary,
	// and finalize the new config.
	if _, ok := attrs[config.UUIDKey]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		attrs[config.UUIDKey] = uuid.String()
	}
	attrs[config.TypeKey] = cloud.Type

	// We need to get the system-identity public key to be added to the
	// newly created model config at AuthorizedKeys.
	// First, we take the controller model's AuthorizedKeys one by one and
	// try to find the one corresponding to system-identity.
	for _, key := range ssh.SplitAuthorisedKeys(base.AuthorizedKeys()) {
		parsedKey, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			logger.Tracef("error parsing controller authorized key %s", key)
			continue
		}
		// system-identity public key must be commented with
		// Juju:juju-system-key
		if parsedKey.Comment == config.JujuSystemKey {
			// If found, add this key to the attrs.
			prevAuthKeys := attrs[config.AuthorizedKeysKey].(string)
			attrs[config.AuthorizedKeysKey] = config.ConcatAuthKeys(prevAuthKeys, key)
			break
		}
	}

	cfg, err := finalizeConfig(provider, cloud, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (c *ModelConfigCreator) checkVersion(base *config.Config, attrs map[string]interface{}) error {
	baseVersion, ok := base.AgentVersion()
	if !ok {
		return errors.Errorf("agent-version not found in base config")
	}

	// If there is no agent-version specified, use the current version.
	// otherwise we need to check for tools
	value, ok := attrs["agent-version"]
	if !ok {
		attrs["agent-version"] = baseVersion.String()
		return nil
	}
	versionStr, ok := value.(string)
	if !ok {
		return errors.Errorf("agent-version must be a string but has type '%T'", value)
	}
	versionNumber, err := version.Parse(versionStr)
	if err != nil {
		return errors.Trace(err)
	}

	n := versionNumber.Compare(baseVersion)
	switch {
	case n > 0:
		return errors.Errorf(
			"agent-version (%s) cannot be greater than the controller (%s)",
			versionNumber, baseVersion,
		)
	case n == 0:
		// If the version is the same as the base config,
		// then assume tools are available.
		return nil
	case n < 0:
		if c.FindTools == nil {
			return errors.New(
				"agent-version does not match base config, " +
					"and no tools-finder is supplied",
			)
		}
	}

	// Look to see if we have tools available for that version.
	list, err := c.FindTools(versionNumber)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	if len(list) == 0 {
		return errors.Errorf("no agent binaries found for version %s", versionNumber)
	}
	logger.Tracef("found agent binaries: %#v", list)
	return nil
}

// finalizeConfig creates the config object from attributes,
// and calls EnvironProvider.PrepareConfig.
func finalizeConfig(
	provider environs.EnvironProvider,
	cloud environscloudspec.CloudSpec,
	attrs map[string]interface{},
) (*config.Config, error) {
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		return nil, errors.Annotate(err, "creating config from values failed")
	}
	cfg, err = provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  cloud,
		Config: cfg,
	})
	if err != nil {
		return nil, errors.Annotate(err, "provider config preparation failed")
	}
	cfg, err = provider.Validate(cfg, nil)
	if err != nil {
		return nil, errors.Annotate(err, "provider config validation failed")
	}
	return cfg, nil
}
