// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package validators

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// CharmhubURLChange returns a config validator that will check to make sure
// the charm hub url has not changed.
func CharmhubURLChange() config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		if v, has := cfg.CharmHubURL(); has {
			oldURL, _ := old.CharmHubURL()
			if v != oldURL {
				return cfg, &config.ValidationError{
					InvalidAttrs: []string{config.CharmHubURLKey},
					Reason:       "charmhub-url cannot be changed",
				}
			}
		}
		return cfg, nil
	}
}

// AgentVersionChange returns a config validator that will check to make sure
// the agent version does not change and also remove it from config so that it
// does not get committed back to state.
//
// Agent version is an ongoing value that is being actively removed from model
// config. Until we can finish removing all uses of agent version from model
// config this validator will keep removing it from the new config so that it
// does not get persisted to state.
func AgentVersionChange() config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		if v, has := cfg.AgentVersion(); has {
			oldVersion, has := old.AgentVersion()
			if !has {
				return cfg, nil
			}
			if v.Compare(oldVersion) != 0 {
				return cfg, &config.ValidationError{
					InvalidAttrs: []string{config.AgentVersionKey},
					Reason:       "agent-version cannot be changed",
				}
			}
		}

		cfg, err := cfg.Remove([]string{config.AgentVersionKey})
		if err != nil {
			return cfg, fmt.Errorf("removing agent version key from model config: %w", err)
		}
		return cfg, nil
	}
}

// AuthorizedKeysChange checks to see if there has been any change to a model
// config authorized keys.
func AuthorizedKeysChange() config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		if cfg.AuthorizedKeys() == old.AuthorizedKeys() {
			// No change. Nothing more todo.
			return cfg, nil
		}

		return cfg, &config.ValidationError{
			InvalidAttrs: []string{config.AuthorizedKeysKey},
			Reason:       "authorized-keys cannot be changed",
		}
	}
}

// SpaceProvider is responsible for checking if a given space exists.
type SpaceProvider interface {
	// HasSpace checks if the supplied space exists within the controller. If
	// during the course of checking for space existence false and an error will
	// be returned.
	HasSpace(context.Context, string) (bool, error)
}

// SpaceChecker will validate a model config's space to see if it exists within
// this Juju controller. Should the space not exist an error satisfying
// config.ValidationError will be returned.
func SpaceChecker(provider SpaceProvider) config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		spaceName := cfg.DefaultSpace()
		if spaceName == "" {
			// No need to verify if the space isn't defined
			return cfg, nil
		}

		has, err := provider.HasSpace(ctx, spaceName)
		if err != nil {
			return cfg, fmt.Errorf("checking for space %q existence to validate model config: %w", spaceName, err)
		}

		if !has {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.DefaultSpaceKey},
				Reason:       fmt.Sprintf("space %q does not exist", spaceName),
			}
		}

		return cfg, nil
	}
}

const (
	// ErrorLogTracingPermission is a specific error to indicate that trace
	// level logging cannot be enabled within model config because the user
	// requesting the change does not have adequate permission.
	ErrorLogTracingPermission = errors.ConstError("permission denied setting log level to tracing")
)

// LoggingTracePermissionChecker checks the logging config for both validity and
// the existence of trace level debugging. If the logging config contains trace
// level logging and the canTrace is set to false we error with an error that
// satisfies both ErrorLogTracingPermission and config.ValidationError.
func LoggingTracePermissionChecker(canTrace bool) config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		// If we can trace no point in checking to see if we having tracing.
		if canTrace {
			return cfg, nil
		}

		rawLogConf := cfg.LoggingConfig()
		logCfg, err := loggo.ParseConfigString(rawLogConf)
		if err != nil {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.LoggingConfigKey},
				Reason:       fmt.Sprintf("failed to parse logging config %q: %v", rawLogConf, err),
			}
		}

		haveTrace := false
		for _, level := range logCfg {
			haveTrace = level == loggo.TRACE
			if haveTrace {
				break
			}
		}
		// No TRACE level requested, so no need to permission check.
		if !haveTrace {
			return cfg, nil
		}

		if !canTrace && haveTrace {
			return cfg, fmt.Errorf(
				"%w %w",
				ErrorLogTracingPermission,
				&config.ValidationError{
					InvalidAttrs: []string{config.LoggingConfigKey},
				},
			)
		}

		return cfg, nil
	}
}

// SecretBackendChecker is responsible for asserting the secret backend in the
// updated model config is a valid secret backend in the controller. If the
// secret backend has not changed or is the default backend then no validation
// is performed. Any validation errors will satisfy config.ValidationError.
func SecretBackendChecker(modelType coremodel.ModelType) config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		backendName := cfg.SecretBackend()
		if backendName == old.SecretBackend() {
			return cfg, nil
		}
		if backendName == "" {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.SecretBackendKey},
				Reason:       "secret back cannot be empty",
			}
		}
		if backendName == config.DefaultSecretBackend {
			return cfg, nil
		}
		if modelType == coremodel.CAAS && backendName == juju.BackendName {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.SecretBackendKey},
				Reason:       `caas secret backend cannot be set to "internal"`,
			}
		}
		if modelType == coremodel.IAAS && backendName == kubernetes.BackendName {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.SecretBackendKey},
				Reason:       `iaas secret backend cannot be set to "kubernetes"`,
			}
		}
		return cfg, nil
	}
}
