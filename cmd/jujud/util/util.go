package util

import (
	"fmt"
	"github.com/juju/errors"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/loggo"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	DataDir           = paths.MustSucceed(paths.DataDir(version.Current.Series))
	EnsureMongoServer = mongo.EnsureServer
)

// requiredError is useful when complaining about missing command-line options.
func RequiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

func IsFatal(err error) bool {
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

type FatalError struct {
	Err string
}

func (e *FatalError) Error() string {
	return e.Err
}

func importance(err error) int {
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

// agentDone processes the error returned by
// an exiting agent.
func AgentDone(logger loggo.Logger, err error) error {
	switch err {
	case worker.ErrTerminateAgent, worker.ErrRebootMachine, worker.ErrShutdownMachine:
		err = nil
	}
	if ug, ok := err.(*upgrader.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let upstart deal with the restart.
			err = errors.Annotate(err, "cannot change agent tools")
			logger.Infof(err.Error())
			return err
		}
	}
	return err
}

type Pinger interface {
	Ping() error
}

// connectionIsFatal returns a function suitable for passing
// as the isFatal argument to worker.NewRunner,
// that diagnoses an error as fatal if the connection
// has failed or if the error is otherwise fatal.
func ConnectionIsFatal(logger loggo.Logger, conn Pinger) func(err error) bool {
	return func(err error) bool {
		if IsFatal(err) {
			return true
		}
		return ConnectionIsDead(logger, conn)
	}
}

// connectionIsDead returns true if the given pinger fails to ping.
var ConnectionIsDead = func(logger loggo.Logger, conn Pinger) bool {
	if err := conn.Ping(); err != nil {
		logger.Infof("error pinging %T: %v", conn, err)
		return true
	}
	return false
}

func SwitchProcessToRollingLogs(logger *lumberjack.Logger) error {
	writer := loggo.NewSimpleWriter(logger, &loggo.DefaultFormatter{})
	_, err := loggo.ReplaceDefaultWriter(writer)
	return err
}
