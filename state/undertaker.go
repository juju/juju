package state

import (
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/errors"
	"github.com/juju/juju/status"

	"gopkg.in/mgo.v2/txn"
)

var ErrModelNotDying = errors.New("model is not dying")

// ProcessDyingModel checks if there are any machines or services left in
// state. If there are none, the model's life is changed from dying to dead.
func (st *State) ProcessDyingModel() (err error) {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	statusDoc := setStatusDoc(setStatusParams{
		badge:     "model",
		globalKey: model.globalKey(),
		status:    status.StatusArchived,
	})
	probablyUpdateStatusHistory(st, model.globalKey(), statusDoc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := model.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		if model.Life() != Dying {
			return nil, errors.Trace(ErrModelNotDying)
		}

		if st.IsController() {
			models, err := st.AllModels()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, model := range models {
				if model.UUID() != st.ModelUUID() && model.Life() != Dead {
					return nil, errors.Errorf("one or more hosted models are not yet dead")
				}
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
				"time-of-death": nowToTheSecond(),
			}},
		}}
		ops = append(ops, decHostedModelCountOp())
		ops = append(ops, updateStatusOp(model.globalKey(), statusDoc, txn.DocExists))

		return ops, nil
	}

	if err := st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	if err := model.SetStatus(status.StatusArchived, "", nil); err != nil {
		return errors.Annotate(err, "updating model status")
	}

	return nil
}
