// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityfilewriter

import (
	"context"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	coreagent "github.com/juju/juju/core/agent"
	internallogger "github.com/juju/juju/internal/logger"
	jworker "github.com/juju/juju/internal/worker"
)

var logger = internallogger.GetLogger("juju.worker.identityfilewriter")

// LegacyManifoldConfig defines the names of the manifolds on which a
// LegacyManifold will depend.
//
// TODO(JUJU-9720): Remove once the controller manifolds are removed from
// jujuagentd in Stage 4.
type LegacyManifoldConfig engine.AgentAPIManifoldConfig

// LegacyManifold returns a dependency manifold that runs an identity file
// writer worker for the jujuagentd transitional path, using the resource
// names defined in the supplied config.
//
// TODO(JUJU-9720): Remove once the controller manifolds are removed from
// jujuagentd in Stage 4.
func LegacyManifold(config LegacyManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newLegacyWorker)
}

// newLegacyWorker wraps NewLegacyWorker for use in a engine.AgentAPIManifold.
func newLegacyWorker(ctx context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	cfg := a.CurrentConfig()

	// Grab the tag and ensure that it's for a controller.
	if !coreagent.IsAllowedControllerTag(cfg.Tag().Kind()) {
		return nil, errors.New("this manifold may only be used inside a machine or controller agent")
	}

	isController, err := apiagent.IsController(ctx, apiCaller, cfg.Tag())
	if err != nil {
		return nil, err
	}
	if !isController {
		return nil, dependency.ErrMissing
	}

	return NewLegacyWorker(cfg)
}

// NewLegacyWorker is the constructor for the jujuagentd transitional SSH
// identity file writer.
//
// TODO(JUJU-9720): Remove once the controller manifolds are removed from
// jujuagentd in Stage 4.
var NewLegacyWorker = func(agentConfig agent.Config) (worker.Worker, error) {
	inner := func(ctx context.Context) error {
		return agent.WriteSystemIdentityFile(agentConfig)
	}
	return jworker.NewSimpleWorker(inner), nil
}

// SystemIdentityValues are the current system identity values used by the
// jujud-only worker when the manifold starts.
type SystemIdentityValues struct {
	SystemIdentity     string
	SystemIdentityPath string
}

// SystemIdentityReader returns the current system identity values when the
// manifold starts.
type SystemIdentityReader interface {
	SystemIdentityValues() (SystemIdentityValues, error)
}

// ManifoldConfig defines the explicit controller-owned inputs needed to write
// the controller system identity file in the jujud-only path. The reader is
// evaluated when the manifold starts so bounced workers observe current
// values without reintroducing broad agent dependencies.
type ManifoldConfig struct {
	// SystemIdentityReader returns the current identity values.
	SystemIdentityReader SystemIdentityReader

	// NewWorker is the constructor for the identity file writer worker. It
	// is exported for injection in unit tests.
	NewWorker func(cfg ManifoldConfig) (worker.Worker, error)
}

// Validate returns an error if the config is incomplete.
func (cfg ManifoldConfig) Validate() error {
	if cfg.SystemIdentityReader == nil {
		return errors.NotValidf("nil SystemIdentityReader")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Validate returns an error if the values are incomplete.
func (v SystemIdentityValues) Validate() error {
	if v.SystemIdentityPath == "" {
		return errors.NotValidf("empty SystemIdentityPath")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the jujud-only SSH identity
// file writer. Unlike LegacyManifold, it does not depend on api-caller and
// does not call apiagent.IsController, because jujud is always the controller
// application.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			values, err := config.SystemIdentityReader.SystemIdentityValues()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if err := values.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			config := config
			config.SystemIdentityReader = staticSystemIdentityReader{values: values}
			return config.NewWorker(config)
		},
	}
}

type staticSystemIdentityReader struct {
	values SystemIdentityValues
}

func (r staticSystemIdentityReader) SystemIdentityValues() (SystemIdentityValues, error) {
	return r.values, nil
}

// NewWorker is the default constructor for the jujud-only SSH identity file
// writer worker. It writes or removes the system identity file based on the
// SystemIdentity value in cfg.
var NewWorker = func(cfg ManifoldConfig) (worker.Worker, error) {
	values, err := cfg.SystemIdentityReader.SystemIdentityValues()
	if err != nil {
		return nil, errors.Trace(err)
	}
	inner := func(ctx context.Context) error {
		return writeSystemIdentityFile(values.SystemIdentity, values.SystemIdentityPath)
	}
	return jworker.NewSimpleWorker(inner), nil
}

// writeSystemIdentityFile writes identity to path with 0600 permissions.
// If identity is empty the file is removed instead. A missing file is not
// an error.
func writeSystemIdentityFile(identity, path string) error {
	if identity != "" {
		logger.Infof(context.TODO(), "writing system identity file")
		if err := utils.AtomicWriteFile(path, []byte(identity), 0600); err != nil {
			return errors.Annotate(err, "cannot write system identity")
		}
		return nil
	}
	logger.Infof(context.TODO(), "removing system identity file")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.Annotate(err, "cannot remove system identity")
	}
	return nil
}
