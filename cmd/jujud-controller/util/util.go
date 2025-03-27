// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/mongo"
	jworker "github.com/juju/juju/internal/worker"
)

// EnsureMongoServerInstalled is patched for testing.
var EnsureMongoServerInstalled = mongo.EnsureServerInstalled

// AgentDone processes the error returned by an exiting agent.
func AgentDone(logger logger.Logger, err error) error {
	err = errors.Cause(err)
	switch err {
	case jworker.ErrTerminateAgent, jworker.ErrRebootMachine, jworker.ErrShutdownMachine:
		// These errors are swallowed here because we want to exit
		// the agent process without error, to avoid the init system
		// restarting us.
		err = nil
	}
	if ug, ok := err.(*agenterrors.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(logger); err != nil {
			// Return and let the init system deal with the restart.
			err = errors.Annotate(err, "cannot change agent binaries")
			logger.Infof(context.TODO(), err.Error())
			return err
		}
	}
	if err == jworker.ErrRestartAgent {
		logger.Warningf(context.TODO(), "agent restarting")
	}
	return err
}

// NewEnsureMongoParams creates a mongo.EnsureServerParams from an agent
// configuration.
var NewEnsureMongoParams = newEnsureMongoParams

func newEnsureMongoParams(agentConfig agent.Config) (mongo.EnsureServerParams, error) {
	// If oplog size is specified in the agent configuration, use that.
	// Otherwise leave the default zero value to indicate to EnsureServer
	// that it should calculate the size.
	var oplogSize int
	if oplogSizeString := agentConfig.Value(agent.MongoOplogSize); oplogSizeString != "" {
		var err error
		if oplogSize, err = strconv.Atoi(oplogSizeString); err != nil {
			return mongo.EnsureServerParams{}, fmt.Errorf("invalid oplog size: %q", oplogSizeString)
		}
	}

	// If numa ctl preference is specified in the agent configuration, use that.
	// Otherwise leave the default false value to indicate to EnsureServer
	// that numactl should not be used.
	var numaCtlPolicy bool
	if numaCtlString := agentConfig.Value(agent.NUMACtlPreference); numaCtlString != "" {
		var err error
		if numaCtlPolicy, err = strconv.ParseBool(numaCtlString); err != nil {
			return mongo.EnsureServerParams{}, fmt.Errorf("invalid numactl preference: %q", numaCtlString)
		}
	}

	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return mongo.EnsureServerParams{}, fmt.Errorf("agent config has no state serving info")
	}

	params := mongo.EnsureServerParams{
		APIPort:        si.APIPort,
		StatePort:      si.StatePort,
		Cert:           si.Cert,
		PrivateKey:     si.PrivateKey,
		CAPrivateKey:   si.CAPrivateKey,
		SharedSecret:   si.SharedSecret,
		SystemIdentity: si.SystemIdentity,

		OplogSize:            oplogSize,
		SetNUMAControlPolicy: numaCtlPolicy,

		JujuDBSnapChannel: agentConfig.JujuDBSnapChannel(),
	}
	return params, nil
}
