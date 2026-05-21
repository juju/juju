// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"time"

	"github.com/juju/gnuflag"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	agentengine "github.com/juju/juju/agent/engine"
	"github.com/juju/juju/caas"
	agentmodel "github.com/juju/juju/cmd/jujud/agent/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/upgrades"
)

var logger = internallogger.GetLogger("juju.cmd.jujud")

// Package-level vars allow tests to intercept calls to these functions.
var (
	newEnvirons   = environs.New
	newCAASBroker = caas.New

	caasModelManifolds = agentmodel.CAASManifolds
	iaasModelManifolds = agentmodel.IAASManifolds
)

type (
	// PreUpgradeStepsFunc is the type of a function used to run pre-upgrade
	// steps.
	PreUpgradeStepsFunc func(coremodel.ModelType) upgrades.PreUpgradeStepsFunc
	// UpgradeStepsFunc is the type of a function used to run upgrade steps.
	UpgradeStepsFunc = upgrades.UpgradeStepsFunc
)

// AgentInitializer handles initializing a type for use as a Jujud
// agent.
type AgentInitializer interface {
	AddFlags(*gnuflag.FlagSet)
	CheckArgs([]string) error
	// DataDir returns the directory where this agent should store its
	// data.
	DataDir() string
}

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter.
func (a *noopStatusSetter) SetStatus(_ context.Context, _ status.Status, _ string, _ map[string]any) error {
	return nil
}

func applyTestingOverrides(agentConfig agent.Config, manifoldsCfg *agentmodel.ManifoldsConfig) {
	if v := agentConfig.Value(agent.CharmRevisionUpdateInterval); v != "" {
		charmRevisionUpdateInterval, err := time.ParseDuration(v)
		if err == nil {
			manifoldsCfg.CharmRevisionUpdateInterval = charmRevisionUpdateInterval
			logger.Infof(context.TODO(), "model worker charm revision update interval set to %v for testing",
				charmRevisionUpdateInterval)
		} else {
			logger.Warningf(context.TODO(), "invalid charm revision update interval, using default %v: %v",
				manifoldsCfg.CharmRevisionUpdateInterval, err)
		}
	}
}

type modelWorker struct {
	*dependency.Engine
	modelUUID string
	metrics   agentengine.MetricSink
}

// Wait is the last thing that is called on the worker as it is being
// removed.
func (m *modelWorker) Wait() error {
	err := m.Engine.Wait()
	// When closing the model, ensure that we also close the metrics
	// with the logger.
	_ = m.metrics.Unregister()
	return err
}
