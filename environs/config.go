// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.environs")

// environ holds information about one environment.
type environ struct {
	config *config.Config
	err    error // an error if the config data could not be parsed.
}

// Environs holds information about each named environment
// in an environments.yaml file.
type Environs struct {
	Default     string // The name of the default environment.
	rawEnvirons map[string]map[string]interface{}
}

// Names returns the list of environment names.
func (e *Environs) Names() (names []string) {
	for name := range e.rawEnvirons {
		names = append(names, name)
	}
	return
}

func validateEnvironmentKind(rawEnviron map[string]interface{}) error {
	kind, _ := rawEnviron["type"].(string)
	if kind == "" {
		return fmt.Errorf("environment %q has no type", rawEnviron["name"])
	}
	p, _ := Provider(kind)
	if p == nil {
		return fmt.Errorf("environment %q has an unknown provider type %q", rawEnviron["name"], kind)
	}
	return nil
}

// disallowedWithNew holds those attributes
// that can not be set in an initial environment
// config used to bootstrap (they must only be set
// on a running environment where appropriate
// validation can be performed).
var disallowedWithBootstrap = []string{
	config.StorageDefaultBlockSourceKey,
}

// Config returns the environment configuration for the environment
// with the given name. If the configuration is not
// found, an errors.NotFoundError is returned.
func (envs *Environs) Config(name string) (*config.Config, error) {
	if name == "" {
		name = envs.Default
		if name == "" {
			return nil, errors.New("no default environment found")
		}
	}
	attrs, ok := envs.rawEnvirons[name]
	if !ok {
		return nil, errors.NotFoundf("environment %q", name)
	}
	if err := validateEnvironmentKind(attrs); err != nil {
		return nil, errors.Trace(err)
	}

	// Check that we don't have any disallowed fields in new configs used for bootstrap.
	for _, attr := range disallowedWithBootstrap {
		if _, ok := attrs[attr]; ok {
			return nil, fmt.Errorf("attribute %q is not allowed in bootstrap configurations", attr)
		}
	}

	// If deprecated config attributes are used, log warnings so the user can know
	// that they need to be fixed.
	// We also look up what any new values might be so we can tell the user.
	newAttrs := config.ProcessDeprecatedAttributes(attrs)
	envs.logDeprecatedWarnings(attrs, newAttrs, config.ToolsMetadataURLKey, config.AgentMetadataURLKey)

	// null has been renamed to manual (with an alias for existing config).
	if oldType, _ := attrs["type"].(string); oldType == "null" {
		logger.Warningf(
			"Provider type \"null\" has been renamed to \"manual\".\n" +
				"Please update your environment configuration.",
		)
	}
	// lxc-use-clone has been renamed to lxc-clone
	envs.logDeprecatedWarnings(attrs, newAttrs, config.LxcUseClone, config.LxcClone)

	// provisioner-safe-mode has been renamed to provisioner-harvest-mode, so log warnings to the user
	envs.logDeprecatedWarnings(attrs, newAttrs, config.ProvisionerSafeModeKey, config.ProvisionerHarvestModeKey)

	// tools-stream has been renamed to agent-stream, so log warnings to the user
	envs.logDeprecatedWarnings(attrs, newAttrs, config.ToolsStreamKey, config.AgentStreamKey)

	// Block attributes only matter if they have been used
	envs.logBlockDeprecationWarnings(attrs)

	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (envs *Environs) logBlockDeprecationWarnings(attrs map[string]interface{}) {
	checkBlockVar := func(key string) {
		if used, ok := attrs[key]; ok {
			if used.(bool) {
				envs.logBlockWarning(key, used)
			}
		}
	}
	checkBlockVar(config.PreventDestroyEnvironmentKey)
	checkBlockVar(config.PreventRemoveObjectKey)
	checkBlockVar(config.PreventAllChangesKey)
}

func (envs *Environs) logBlockWarning(key string, value interface{}) {
	logger.Warningf(
		"Config attribute %q (%v) is deprecated and will be ignored since \n"+
			"upgrade process takes care of it. \n"+
			"The attribute %q should be removed from your configuration.",
		key, value, key,
	)
}

// logDeprecatedWarnings constructs log warning messages for deprecated attributes names.
// It checks if both old and new attribute names are provided.
// When both are provided, the message warns to remove old attribute from configuration.
// When only old attribute name is used, the message advises to replace it with the new name.
func (envs *Environs) logDeprecatedWarnings(attrs, newAttrs map[string]interface{}, oldKey, newKey string) {
	if oldValue := attrs[oldKey]; oldValue != nil {
		// no need to warn if attribute is unused
		if oldStr, ok := oldValue.(string); ok && oldStr == "" {
			return
		}
		newValue, newValueSpecified := attrs[newKey]
		var msg string
		if newValueSpecified {
			msg = fmt.Sprintf(
				"Config attribute %q (%v) is deprecated and will be ignored since \n"+
					"the new %q (%v) attribute has also been used. \n"+
					"The attribute %q should be removed from your configuration.",
				oldKey, oldValue, newKey, newValue, oldKey)
		} else {
			msg = fmt.Sprintf(
				"Config attribute %q (%v) is deprecated. \n"+
					"It is replaced by %q attribute. \n"+
					"Your configuration should be updated to set %q as follows \n%v: %v.",
				oldKey, oldValue, newKey, newKey, newKey, newAttrs[newKey])
		}
		logger.Warningf(msg)
	}
}

// providers maps from provider type to EnvironProvider for
// each registered provider type.
//
// providers should not typically be used directly; the
// Provider function will handle provider type aliases,
// and should be used instead.
var providers = make(map[string]EnvironProvider)

// providerAliases is a map of provider type aliases.
var providerAliases = make(map[string]string)

// RegisterProvider registers a new environment provider. Name gives the name
// of the provider, and p the interface to that provider.
//
// RegisterProvider will panic if the provider name or any of the aliases
// are registered more than once.
func RegisterProvider(name string, p EnvironProvider, alias ...string) {
	if providers[name] != nil || providerAliases[name] != "" {
		panic(errors.Errorf("juju: duplicate provider name %q", name))
	}
	providers[name] = p
	for _, alias := range alias {
		if providers[alias] != nil || providerAliases[alias] != "" {
			panic(errors.Errorf("juju: duplicate provider alias %q", alias))
		}
		providerAliases[alias] = name
	}
}

// RegisteredProviders enumerate all the environ providers which have been registered.
func RegisteredProviders() []string {
	var p []string
	for k := range providers {
		p = append(p, k)
	}
	return p
}

// Provider returns the previously registered provider with the given type.
func Provider(providerType string) (EnvironProvider, error) {
	if alias, ok := providerAliases[providerType]; ok {
		providerType = alias
	}
	p, ok := providers[providerType]
	if !ok {
		return nil, errors.Errorf("no registered provider for %q", providerType)
	}
	return p, nil
}

// ReadEnvironsBytes parses the contents of an environments.yaml file
// and returns its representation. An environment with an unknown type
// will only generate an error when New is called for that environment.
// Attributes for environments with known types are checked.
func ReadEnvironsBytes(data []byte) (*Environs, error) {
	var raw struct {
		Default      string
		Environments map[string]map[string]interface{}
	}
	err := goyaml.Unmarshal(data, &raw)
	if err != nil {
		return nil, err
	}

	if raw.Default != "" && raw.Environments[raw.Default] == nil {
		return nil, errors.Errorf("default environment %q does not exist", raw.Default)
	}
	if raw.Default == "" {
		// If there's a single environment, then we get the default
		// automatically.
		if len(raw.Environments) == 1 {
			for name := range raw.Environments {
				raw.Default = name
				break
			}
		}
	}
	for name, attrs := range raw.Environments {
		// store the name of the this environment in the config itself
		// so that providers can see it.
		attrs["name"] = name
	}
	return &Environs{raw.Default, raw.Environments}, nil
}

func environsPath(path string) string {
	if path == "" {
		path = osenv.JujuHomePath("environments.yaml")
	}
	return path
}

// NoEnvError indicates the default environment config file is missing.
type NoEnvError struct {
	error
}

// IsNoEnv reports whether err is a NoEnvError.
func IsNoEnv(err error) bool {
	_, ok := err.(NoEnvError)
	return ok
}

// ReadEnvirons reads the juju environments.yaml file
// and returns the result of running ParseEnvironments
// on the file's contents.
// If path is empty, $HOME/.juju/environments.yaml is used.
func ReadEnvirons(path string) (*Environs, error) {
	environsFilepath := environsPath(path)
	data, err := ioutil.ReadFile(environsFilepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NoEnvError{err}
		}
		return nil, err
	}
	e, err := ReadEnvironsBytes(data)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q: %v", environsFilepath, err)
	}
	return e, nil
}

// WriteEnvirons creates a new juju environments.yaml file with the specified contents.
func WriteEnvirons(path string, fileContents string) (string, error) {
	environsFilepath := environsPath(path)
	environsDir := filepath.Dir(environsFilepath)
	var info os.FileInfo
	var err error
	if info, err = os.Lstat(environsDir); os.IsNotExist(err) {
		if err = os.MkdirAll(environsDir, 0700); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if info.Mode().Perm() != 0700 {
		logger.Warningf("permission of %q is %q", environsDir, info.Mode().Perm())
	}
	if err := ioutil.WriteFile(environsFilepath, []byte(fileContents), 0600); err != nil {
		return "", err
	}
	// WriteFile does not change permissions of existing files.
	if err := os.Chmod(environsFilepath, 0600); err != nil {
		return "", err
	}
	return environsFilepath, nil
}

// BootstrapConfig returns a copy of the supplied configuration with the
// admin-secret and ca-private-key attributes removed. If the resulting
// config is not suitable for bootstrapping an environment, an error is
// returned.
func BootstrapConfig(cfg *config.Config) (*config.Config, error) {
	m := cfg.AllAttrs()
	// We never want to push admin-secret or the root CA private key to the cloud.
	delete(m, "admin-secret")
	delete(m, "ca-private-key")
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		return nil, err
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return nil, fmt.Errorf("environment configuration has no agent-version")
	}
	return cfg, nil
}
