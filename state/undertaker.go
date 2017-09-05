// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var ErrModelNotDying = errors.New("model is not dying")

// ProcessDyingModel checks if the model is Dying and empty, and if so,
// transitions the model to Dead.
//
// If the model is non-empty because it is the controller model and still
// contains hosted models, an error satisfying IsHasHostedModelsError will
// be returned. If the model is otherwise non-empty, an error satisfying
// IsNonEmptyModelError will be returned.
func (st *State) ProcessDyingModel() (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if model.Life() != Dying {
			return nil, errors.Trace(ErrModelNotDying)
		}

		if st.IsController() {
			// We should not mark the controller model as Dead until
			// all hosted models have been removed, otherwise the
			// hosted model environs may not have beeen destroyed.
			models, err := st.AllModels()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if n := len(models) - 1; n > 0 {
				return nil, errors.Trace(hasHostedModelsError(n))
			}
		}

		if err := model.checkEmpty(); err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      modelsC,
			Id:     st.ModelUUID(),
			Assert: isDyingDoc,
			Update: bson.M{"$set": bson.M{
				"life":          Dead,
				"time-of-death": st.NowToTheSecond(),
			}},
		}, {
			// Cleanup the owner:envName unique key.
			C:      usermodelnameC,
			Id:     model.uniqueIndexID(),
			Remove: true,
		}}
		return ops, nil
	}

	if err = st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
