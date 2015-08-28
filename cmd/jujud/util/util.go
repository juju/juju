// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/agent"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/upgrader"
)

var (
	logger            = loggo.GetLogger("juju.cmd.jujud.util")
	DataDir           = paths.MustSucceed(paths.DataDir(version.Current.Series))
	EnsureMongoServer = mongo.EnsureServer
)

// requiredError is useful when complaining about missing command-line options.
func RequiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// IsFatal determines if an error is fatal to the process.
func IsFatal(err error) bool {
	err = errors.Cause(err)
	switch err {
	case worker.ErrTerminateAgent, worker.ErrRebootMachine, worker.ErrShutdownMachine:
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
	case err == worker.ErrRebootMachine:
		return 3
	case err == worker.ErrShutdownMachine:
		return 3
	case err == worker.ErrTerminateAgent:
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

// AgentDone processes the error returned by
// an exiting agent.
func AgentDone(logger loggo.Logger, err error) error {
	err = errors.Cause(err)
	switch err {
	case worker.ErrTerminateAgent, worker.ErrRebootMachine, worker.ErrShutdownMachine:
		err = nil
	}
	if ug, ok := err.(*upgrader.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let the init system deal with the restart.
			err = errors.Annotate(err, "cannot change agent tools")
			logger.Infof(err.Error())
			return err
		}
	}
	return err
}

// Pinger provides a type that knows how to ping.
type Pinger interface {

	// Ping pings something.
	Ping() error
}

// ConnectionIsFatal returns a function suitable for passing as the
// isFatal argument to worker.NewRunner, that diagnoses an error as
// fatal if the connection has failed or if the error is otherwise
// fatal.
func ConnectionIsFatal(logger loggo.Logger, conns ...Pinger) func(err error) bool {
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

// ConnectionIsDead returns true if the given pinger fails to ping.
var ConnectionIsDead = func(logger loggo.Logger, conn Pinger) bool {
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
	if numaCtlString := agentConfig.Value(agent.NumaCtlPreference); numaCtlString != "" {
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
		Namespace:            agentConfig.Value(agent.Namespace),
		OplogSize:            oplogSize,
		SetNumaControlPolicy: numaCtlPolicy,
	}
	return params, nil
}

// NewCloseWorker returns a task that wraps the given task,
// closing the given closer when it finishes.
func NewCloseWorker(logger loggo.Logger, worker worker.Worker, closer io.Closer) worker.Worker {
	return &CloseWorker{
		worker: worker,
		closer: closer,
		logger: logger,
	}
}

// CloseWorker is a worker which closes the given closer when finished
// with a task.
type CloseWorker struct {
	worker worker.Worker
	closer io.Closer
	logger loggo.Logger
}

func (c *CloseWorker) Kill() {
	c.worker.Kill()
}

func (c *CloseWorker) Wait() error {
	err := c.worker.Wait()
	if err := c.closer.Close(); err != nil {
		c.logger.Errorf("closeWorker: close error: %v", err)
	}
	return err
}

// HookExecutionLock returns an *fslock.Lock suitable for use as a
// unit hook execution lock. Other workers may also use this lock if
// they require isolation from hook execution.
func HookExecutionLock(dataDir string) (*fslock.Lock, error) {
	lockDir := filepath.Join(dataDir, "locks")
	return fslock.NewLock(lockDir, "uniter-hook-execution")
}

// NewRsyslogConfigWorker creates and returns a new
// RsyslogConfigWorker based on the specified configuration
// parameters.
var NewRsyslogConfigWorker = func(st *apirsyslog.State, agentConfig agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
	tag := agentConfig.Tag()
	namespace := agentConfig.Value(agent.Namespace)
	addrs, err := agentConfig.APIAddresses()
	if err != nil {
		return nil, err
	}
	return rsyslog.NewRsyslogConfigWorker(st, mode, tag, namespace, addrs, agent.DefaultConfDir)
}

// ParamsStateServingInfoToStateStateServingInfo converts a
// params.StateServingInfo to a state.StateServingInfo.
func ParamsStateServingInfoToStateStateServingInfo(i params.StateServingInfo) state.StateServingInfo {
	return state.StateServingInfo{
		APIPort:        i.APIPort,
		StatePort:      i.StatePort,
		Cert:           i.Cert,
		PrivateKey:     i.PrivateKey,
		CAPrivateKey:   i.CAPrivateKey,
		SharedSecret:   i.SharedSecret,
		SystemIdentity: i.SystemIdentity,
	}
}
