package state

import (
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/errors"

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
			logger.Debugf("%v", model.Life())
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
		return ops, nil
	}

	if err = st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
