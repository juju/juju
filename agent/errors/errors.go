// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/version"
	jworker "github.com/juju/juju/internal/worker"
)

// UpgradeReadyError is returned by an Upgrader to report that
// an upgrade is ready to be performed and a restart is due.
type UpgradeReadyError struct {
	AgentName string
	OldTools  version.Binary
	NewTools  version.Binary
	DataDir   string
}

const (
	// FatalError is an error type designated for fatal errors.
	FatalError = errors.ConstError("fatal")
)

func (e *UpgradeReadyError) Error() string {
	return "must restart: an agent upgrade is available"
}

// ChangeAgentTools does the actual agent upgrade.
// It should be called just before an agent exits, so that
// it will restart running the new tools.
func (e *UpgradeReadyError) ChangeAgentTools(logger logger.Logger) error {
	agentTools, err := tools.ChangeAgentTools(e.DataDir, e.AgentName, e.NewTools)
	if err != nil {
		return err
	}
	logger.Infof(context.TODO(), "upgraded from %v to %v (%q)", e.OldTools, agentTools.Version, agentTools.URL)
	return nil
}

// RequiredError is useful when complaining about missing command-line options.
func RequiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// IsFatal determines if an error is fatal to the process.
func IsFatal(err error) bool {
	if errors.Is(err, jworker.ErrTerminateAgent) ||
		errors.Is(err, jworker.ErrRebootMachine) ||
		errors.Is(err, jworker.ErrShutdownMachine) ||
		errors.Is(err, jworker.ErrRestartAgent) ||
		errors.Is(err, FatalError) ||
		isUpgraded(err) {
		return true
	}
	return false
}

// isUpgraded returns true if error contains an UpgradeReadyError within it's
// chain.
func isUpgraded(err error) bool {
	_, is := errors.AsType[*UpgradeReadyError](err)
	return is
}

func importance(err error) int {
	switch {
	case err == nil:
		return 0
	case isUpgraded(err):
		return 2
	case errors.Is(err, jworker.ErrRebootMachine):
		return 3
	case errors.Is(err, jworker.ErrShutdownMachine):
		return 3
	case errors.Is(err, jworker.ErrTerminateAgent):
		return 4
	default:
		return 1
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

// Breakable provides a type that exposes an IsBroken check.
type Breakable interface {
	IsBroken(ctx context.Context) bool
}

// ConnectionIsFatal returns a function suitable for passing as the
// isFatal argument to worker.NewRunner, that diagnoses an error as
// fatal if the connection has failed or if the error is otherwise
// fatal.
func ConnectionIsFatal(ctx context.Context, logger logger.Logger, conns ...Breakable) func(err error) bool {
	return func(err error) bool {
		if IsFatal(err) {
			return true
		}
		for _, conn := range conns {
			if ConnectionIsDead(ctx, logger, conn) {
				return true
			}
		}
		return false
	}
}

// ConnectionIsDead returns true if the given Breakable is broken.
var ConnectionIsDead = func(ctx context.Context, logger logger.Logger, conn Breakable) bool {
	return conn.IsBroken(ctx)
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
//  1. State grows a Broken() channel like api.Connection (which is
//     actually quite a nice idea).
//  2. The dependency engine conversion is completed for the machine
//     agent.
func PingerIsFatal(logger logger.Logger, conns ...Pinger) func(err error) bool {
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
var PingerIsDead = func(logger logger.Logger, conn Pinger) bool {
	if err := conn.Ping(); err != nil {
		logger.Infof(context.TODO(), "error pinging %T: %v", conn, err)
		return true
	}
	return false
}
