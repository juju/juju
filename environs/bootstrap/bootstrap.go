// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.environs.bootstrap")

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args environs.BootstrapParams) error {
	cfg := environ.Config()
	network.InitializeFromConfig(cfg)
	if secret := cfg.AdminSecret(); secret == "" {
		return errors.Errorf("environment configuration has no admin-secret")
	}
	if authKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys()); len(authKeys) == 0 {
		// Apparently this can never happen, so it's not tested. But, one day,
		// Config will act differently (it's pretty crazy that, AFAICT, the
		// authorized-keys are optional config settings... but it's impossible
		// to actually *create* a config without them)... and when it does,
		// we'll be here to catch this problem early.
		return errors.Errorf("environment configuration has no authorized-keys")
	}
	if _, hasCACert := cfg.CACert(); !hasCACert {
		return errors.Errorf("environment configuration has no ca-cert")
	}
	if _, hasCAKey := cfg.CAPrivateKey(); !hasCAKey {
		return errors.Errorf("environment configuration has no ca-private-key")
	}

	// Write out the bootstrap-init file, and confirm storage is writeable.
	if err := environs.VerifyStorage(environ.Storage()); err != nil {
		return err
	}

	// We must generate an SSH key for the state servers to log into other instances.
	// generateSystemSSHKey updates the environment's authorized-keys, so this must
	// be done before bootstrapping.
	privateKey, err := generateSystemSSHKey(environ)
	if err != nil {
		return err
	}

	ctx.Infof("Bootstrapping environment %q", cfg.Name())
	logger.Debugf("environment %q supports service/machine networks: %v", cfg.Name(), environ.SupportNetworks())

	ctx.Infof("Starting new instance for initial state server")
	arch, series, finalizer, err := environ.Bootstrap(ctx, args)
	if err != nil {
		return err
	}

	// TODO(axw) remove EnsureToolsAvailability calls from providers,
	// make this function responsible for locating tools first and
	// uploading during finalization if necessary.
	availableTools, err := EnsureToolsAvailability(ctx, environ, series, &arch)
	if err != nil {
		return err
	}

	ctx.Infof("Installing Juju agent on bootstrap instance")
	machineConfig, err := environs.NewBootstrapMachineConfig(args.Constraints, privateKey, series)
	if err != nil {
		return err
	}
	machineConfig.Tools = availableTools[0]
	if err := finalizer(ctx, machineConfig); err != nil {
		return err
	}
	ctx.Infof("Bootstrap complete")
	return nil
}

// generateSystemSSHKey creates a new key for the system identity. The
// authorized_keys in the environment config is updated to include the public
// key for the generated key.
func generateSystemSSHKey(env environs.Environ) (privateKey string, err error) {
	logger.Debugf("generate a system ssh key")
	// Create a new system ssh key and add that to the authorized keys.
	privateKey, publicKey, err := ssh.GenerateKey(config.JujuSystemKey)
	if err != nil {
		return "", fmt.Errorf("failed to create system key: %v", err)
	}
	authorized_keys := config.ConcatAuthKeys(env.Config().AuthorizedKeys(), publicKey)
	newConfig, err := env.Config().Apply(map[string]interface{}{
		config.AuthKeysConfig: authorized_keys,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create new config: %v", err)
	}
	if err = env.SetConfig(newConfig); err != nil {
		return "", fmt.Errorf("failed to set new config: %v", err)
	}
	return privateKey, nil
}

// SetBootstrapTools returns the newest tools from the given tools list,
// and updates the agent-version configuration attribute.
func SetBootstrapTools(environ environs.Environ, possibleTools coretools.List) (coretools.List, error) {
	if len(possibleTools) == 0 {
		return nil, fmt.Errorf("no bootstrap tools available")
	}
	var newVersion version.Number
	newVersion, toolsList := possibleTools.Newest()
	logger.Infof("newest version: %s", newVersion)
	cfg := environ.Config()
	if agentVersion, _ := cfg.AgentVersion(); agentVersion != newVersion {
		cfg, err := cfg.Apply(map[string]interface{}{
			"agent-version": newVersion.String(),
		})
		if err == nil {
			err = environ.SetConfig(cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to update environment configuration: %v", err)
		}
	}
	bootstrapVersion := newVersion
	// We should only ever bootstrap the exact same version as the client,
	// or we risk bootstrap incompatibility. We still set agent-version to
	// the newest version, so the agent will immediately upgrade itself.
	if !isCompatibleVersion(newVersion, version.Current.Number) {
		compatibleVersion, compatibleTools := findCompatibleTools(possibleTools, version.Current.Number)
		if len(compatibleTools) == 0 {
			logger.Warningf(
				"failed to find %s tools, will attempt to use %s",
				version.Current.Number, newVersion,
			)
		} else {
			bootstrapVersion, toolsList = compatibleVersion, compatibleTools
		}
	}
	logger.Infof("picked bootstrap tools version: %s", bootstrapVersion)
	return toolsList, nil
}

// findCompatibleTools finds tools in the list that have the same major, minor
// and patch level as version.Current.
//
// Build number is not important to match; uploaded tools will have
// incremented build number, and we want to match them.
func findCompatibleTools(possibleTools coretools.List, version version.Number) (version.Number, coretools.List) {
	var compatibleTools coretools.List
	for _, tools := range possibleTools {
		if isCompatibleVersion(tools.Version.Number, version) {
			compatibleTools = append(compatibleTools, tools)
		}
	}
	return compatibleTools.Newest()
}

func isCompatibleVersion(v1, v2 version.Number) bool {
	v1.Build = 0
	v2.Build = 0
	return v1.Compare(v2) == 0
}

// EnsureNotBootstrapped returns nil if the environment is not
// bootstrapped, and an error if it is or if the function was not able
// to tell.
func EnsureNotBootstrapped(env environs.Environ) error {
	_, err := env.StateServerInstances()
	// If there is no error determining state server instaces,
	// then we are bootstrapped.
	if err == nil {
		return environs.ErrAlreadyBootstrapped
	}
	if err == environs.ErrNotBootstrapped {
		return nil
	}
	return err
}
