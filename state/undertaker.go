// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var ErrModelNotDying = errors.New("model is not dying")

// ProcessDyingModel checks if there are any machines or services left in
// state. If there are none, the model's life is changed from dying to dead.
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
			modelUUIDs, err := st.AllModelUUIDs()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, modelUUID := range modelUUIDs {
				dead, err := isDead(st, modelsC, modelUUID)
				if err != nil {
					return nil, errors.Trace(err)
				}
				if modelUUID != st.ModelUUID() && !dead {
					return nil, errors.Errorf("one or more hosted models are not yet dead")
				}
			}
		}

		modelEntityRefsDoc, err := model.getEntityRefs()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if _, err := checkModelEntityRefsEmpty(modelEntityRefsDoc); err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      modelsC,
			Id:     st.ModelUUID(),
			Assert: isDyingDoc,
			Update: bson.M{"$set": bson.M{
				"life":          Dead,
				"time-of-death": st.nowToTheSecond(),
			}},
		}}
		return ops, nil
	}

	if err = st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
