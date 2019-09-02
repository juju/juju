package modelgeneration

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
)

//go:generate mockgen -package mocks -destination mocks/commit_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration CommitBranchModelOp,CommitBranchState,Settings

// CommitBranchModelOp describes a model operation for committing a branch.
type CommitBranchModelOp interface {
	state.ModelOperation

	// GetModelGen returns the new model generation resulting from a
	// successfully committed branch.
	GetModelGen() int
}

// CommitBranchState describes state operations required
// to execute the CommitBranch operation.
// * This allows us to indirect state at the operation level instead of the
// * whole API level as currently done in interface.go
type CommitBranchState interface {
	Branch(name string) (*state.Generation, error)
	Application(string) (*state.Application, error)
	ControllerTimestamp() (*time.Time, error)
}

// Settings describes methods for interacting with settings to apply
// branch-based configuration deltas.
type Settings interface {
	DeltaOps(key string, delta settings.ItemChanges) ([]txn.Op, error)
}

type commitBranchModelOp struct {
	st       CommitBranchState
	br       *state.Generation
	user     string
	apps     map[string]*state.Application
	settings Settings
	newGenId int
}

// Build (state.ModelOperation) creates and returns a slice of transaction
// operations necessary to commit a branch.
func (o *commitBranchModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := o.br.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	if err := o.br.ValidateForCompletion(); err != nil {
		return nil, errors.Trace(err)
	}

	now, err := o.st.ControllerTimestamp()
	if err != nil {
		return nil, errors.Trace(err)
	}

	assigned, err := o.assignedWithAllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops, err := o.commitConfigTxnOps()
	if err != nil {
		return nil, errors.Trace(err)
	}

	completeOps, newGenId, err := o.br.CompleteOps(assigned, now, o.user)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, completeOps...)
	o.newGenId = newGenId

	return ops, nil
}

// Done (state.ModelOperation) simply returns the error
// resulting from the call to `Build`.
func (o *commitBranchModelOp) Done(err error) error {
	return err
}

// GetModelGen (CommitBranchModelOp) returns the new model generation
// resulting from a successfully committed branch.
func (o *commitBranchModelOp) GetModelGen() int {
	return o.newGenId
}

// assignedWithAllUnits generates a new value for the branch's
// AssignedUnits field, to indicate that all units of changed applications
// are tracking the branch.
// Retrieved applications are cached for later use.
func (o *commitBranchModelOp) assignedWithAllUnits() (map[string][]string, error) {
	assigned := o.br.AssignedUnits()
	o.apps = make(map[string]*state.Application, len(assigned))

	for appName := range assigned {
		app, err := o.st.Application(appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		o.apps[appName] = app

		units, err := app.UnitNames()
		if err != nil {
			return nil, errors.Trace(err)
		}
		assigned[appName] = units
	}
	return assigned, nil
}

// commitConfigTxnOps iterates over all the applications with configuration
// deltas, determines their effective new settings, then gathers the
// operations representing the changes so that they can all be applied in a
// single transaction.
func (o *commitBranchModelOp) commitConfigTxnOps() ([]txn.Op, error) {
	var ops []txn.Op
	for appName, delta := range o.br.Config() {
		if len(delta) == 0 {
			continue
		}

		// We know that any application with config changes is present in the
		// branch's assigned units map, so we know it will have been cached.
		appOps, err := o.settings.DeltaOps(o.apps[appName].CharmConfigKey(), delta)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, appOps...)
	}
	return ops, nil
}

// CommitBranch commits the input branch, making its changes applicable to
// the whole model and marking it complete.
func (api *API) CommitBranch(arg params.BranchArg) (params.IntResult, error) {
	result := params.IntResult{}

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	operation, err := api.opFactory.NewCommitBranchModelOp(arg.BranchName, api.apiUser.Name())
	if err != nil {
		return intResultsError(err)
	}

	if err = api.st.ApplyOperation(operation); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Result = operation.GetModelGen()
	}
	return result, nil
}
