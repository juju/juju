// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent/tools"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.caasoperator")

// A CaasOperatorExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type CaasOperatorExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

// CaasOperator implements the capabilities of the caasoperator agent. It is not intended to
// implement the actual *behaviour* of the caasoperator agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the caasoperator's responses to them.
type CaasOperator struct {
	catacomb    catacomb.Catacomb
	clock       clock.Clock
	agentDir    string
	hookToolDir string
	dataDir     string
}

// CaasOperatorParams hold all the necessary parameters for a new CaasOperator.
type CaasOperatorParams struct {
	CaasOperatorTag      names.ApplicationTag
	AgentDir             string
	DataDir              string
	TranslateResolverErr func(error) error
	Clock                clock.Clock
}

// NewCaasOperator creates a new CaasOperator which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewCaasOperator(caasoperatorParams *CaasOperatorParams) (*CaasOperator, error) {
	translateResolverErr := caasoperatorParams.TranslateResolverErr
	if translateResolverErr == nil {
		translateResolverErr = func(err error) error { return err }
	}

	op := &CaasOperator{
		clock:       caasoperatorParams.Clock,
		agentDir:    caasoperatorParams.AgentDir,
		dataDir:     caasoperatorParams.DataDir,
		hookToolDir: filepath.Join(caasoperatorParams.DataDir, "tools"),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &op.catacomb,
		Work: func() error {
			return op.loop(caasoperatorParams.CaasOperatorTag)
		},
	})
	return op, errors.Trace(err)
}

func (op *CaasOperator) loop(applicationTag names.ApplicationTag) (err error) {
	if err := op.init(applicationTag); err != nil {
		if err == jworker.ErrTerminateAgent {
			return err
		}
		return errors.Annotatef(err, "failed to initialize caasoperator for %q", applicationTag)
	}

	for {
		select {
		case <-op.catacomb.Dying():
			return op.catacomb.ErrDying()
		}
	}
}

func (op *CaasOperator) init(caasapplicationtag names.ApplicationTag) (err error) {
	logger.Criticalf("creating caas operator symlinks in %v", op.agentDir)
	if err := tools.EnsureSymlinks(op.agentDir, op.hookToolDir, commands.CommandNames()); err != nil {
		return err
	}
	return nil
}

func (op *CaasOperator) Kill() {
	op.catacomb.Kill(nil)
}

func (op *CaasOperator) Wait() error {
	return op.catacomb.Wait()
}
