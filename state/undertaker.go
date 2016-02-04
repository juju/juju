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
		env, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if env.Life() != Dying {
			return nil, errors.Trace(ErrModelNotDying)
		}

		if st.IsController() {
			envs, err := st.AllModels()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, env := range envs {
				if env.UUID() != st.ModelUUID() && env.Life() != Dead {
					return nil, errors.Errorf("one or more hosted models are not yet dead")
				}
			}
		}

		machines, err := st.AllMachines()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if l := len(machines); l > 0 {
			return nil, errors.Errorf("model not empty, found %d machine(s)", l)
		}
		services, err := st.AllServices()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if l := len(services); l > 0 {
			return nil, errors.Errorf("model not empty, found %d service(s)", l)
		}
		return []txn.Op{{
			C:      modelsC,
			Id:     st.ModelUUID(),
			Assert: isDyingDoc,
			Update: bson.M{"$set": bson.M{
				"life":          Dead,
				"time-of-death": nowToTheSecond(),
			}},
		}}, nil
	}

	if err = st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
