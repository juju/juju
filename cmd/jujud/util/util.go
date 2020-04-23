// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/mongo"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/upgrader"
)

var (
	logger            = loggo.GetLogger("juju.cmd.jujud.util")
	DataDir           = paths.MustSucceed(paths.DataDir(series.MustHostSeries()))
	LogDir            = paths.MustSucceed(paths.LogDir(series.MustHostSeries()))
	EnsureMongoServer = mongo.EnsureServer
)

// RequiredError is useful when complaining about missing command-line options.
func RequiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// IsFatal determines if an error is fatal to the process.
func IsFatal(err error) bool {
	err = errors.Cause(err)
	switch err {
	case jworker.ErrTerminateAgent, jworker.ErrRebootMachine, jworker.ErrShutdownMachine, jworker.ErrRestartAgent:
		return true
	}

	if isUpgraded(err) {
		return true
	}
	_, ok := err.(*FatalError)
	return ok
}

func isUpgraded(err error) bool {
	_, ok := err.(*upgrader.UpgradeReadyError)
	return ok
}

// FatalError is an error type designated for fatal errors.
type FatalError struct {
	Err string
}

// Error returns an error string.
func (e *FatalError) Error() string {
	return e.Err
}

func importance(err error) int {
	err = errors.Cause(err)
	switch {
	case err == nil:
		return 0
	default:
		return 1
	case isUpgraded(err):
		return 2
	case err == jworker.ErrRebootMachine:
		return 3
	case err == jworker.ErrShutdownMachine:
		return 3
	case err == jworker.ErrTerminateAgent:
		return 4
	}
}

// MoreImportant returns whether err0 is more important than err1 -
// that is, whether we should act on err0 in preference to err1.
func MoreImportant(err0, err1 error) bool {
	return importance(err0) > importance(err1)
}

// MoreImportantError returns the most important error
func MoreImportantError(err0, err1 error) error {
	if importance(err0) > importance(err1) {
		return err0
	}
	return err1
}

// AgentDone processes the error returned by an exiting agent.
func AgentDone(logger loggo.Logger, err error) error {
	err = errors.Cause(err)
	switch err {
	case jworker.ErrTerminateAgent, jworker.ErrRebootMachine, jworker.ErrShutdownMachine:
		// These errors are swallowed here because we want to exit
		// the agent process without error, to avoid the init system
		// restarting us.
		err = nil
	}
	if ug, ok := err.(*upgrader.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let the init system deal with the restart.
			err = errors.Annotate(err, "cannot change agent binaries")
			logger.Infof(err.Error())
			return err
		}
	}
	if err == jworker.ErrRestartAgent {
		logger.Warningf("agent restarting")
	}
	return err
}

// Breakable provides a type that exposes an IsBroken check.
type Breakable interface {
	IsBroken() bool
}

// ConnectionIsFatal returns a function suitable for passing as the
// isFatal argument to worker.NewRunner, that diagnoses an error as
// fatal if the connection has failed or if the error is otherwise
// fatal.
func ConnectionIsFatal(logger loggo.Logger, conns ...Breakable) func(err error) bool {
	return func(err error) bool {
		if IsFatal(err) {
			return true
		}
		for _, conn := range conns {
			if ConnectionIsDead(logger, conn) {
				return true
			}
		}
		return false
	}
}

// ConnectionIsDead returns true if the given Breakable is broken.
var ConnectionIsDead = func(logger loggo.Logger, conn Breakable) bool {
	return conn.IsBroken()
}

// Pinger provides a type that knows how to ping.
type Pinger interface {
	Ping() error
}

// PingerIsFatal returns a function suitable for passing as the
// isFatal argument to worker.NewRunner, that diagnoses an error as
// fatal if the Pinger has failed or if the error is otherwise fatal.
//
// TODO(mjs) - this only exists for checking State instances in the
// machine agent and won't be necessary once either:
// 1. State grows a Broken() channel like api.Connection (which is
//    actually quite a nice idea).
// 2. The dependency engine conversion is completed for the machine
//    agent.
func PingerIsFatal(logger loggo.Logger, conns ...Pinger) func(err error) bool {
	return func(err error) bool {
		if IsFatal(err) {
			return true
		}
		for _, conn := range conns {
			if PingerIsDead(logger, conn) {
				return true
			}
		}
		return false
	}
}

// PingerIsDead returns true if the given pinger fails to ping.
var PingerIsDead = func(logger loggo.Logger, conn Pinger) bool {
	if err := conn.Ping(); err != nil {
		logger.Infof("error pinging %T: %v", conn, err)
		return true
	}
	return false
}

// NewEnsureServerParams creates an EnsureServerParams from an agent
// configuration.
func NewEnsureServerParams(agentConfig agent.Config) (mongo.EnsureServerParams, error) {
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

		DataDir:              agentConfig.DataDir(),
		OplogSize:            oplogSize,
		SetNUMAControlPolicy: numaCtlPolicy,

		MemoryProfile:     agentConfig.MongoMemoryProfile(),
		JujuDBSnapChannel: agentConfig.JujuDBSnapChannel(),
	}
	return params, nil
}
