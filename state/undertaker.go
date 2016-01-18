package state

import (
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/errors"

	"gopkg.in/mgo.v2/txn"
)

var ErrEnvironmentNotDying = errors.New("environment is not dying")

// ProcessDyingEnviron checks if there are any machines or services left in
// state. If there are none, the environment's life is changed from dying to dead.
func (st *State) ProcessDyingEnviron() (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		env, err := st.Environment()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if env.Life() != Dying {
			return nil, errors.Trace(ErrEnvironmentNotDying)
		}

		if st.IsStateServer() {
			envs, err := st.AllEnvironments()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, env := range envs {
				if env.UUID() != st.EnvironUUID() && env.Life() != Dead {
					return nil, errors.Errorf("one or more hosted environments are not yet dead")
				}
			}
		}

		machines, err := st.AllMachines()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if l := len(machines); l > 0 {
			return nil, errors.Errorf("environment not empty, found %d machine(s)", l)
		}
		services, err := st.AllServices()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if l := len(services); l > 0 {
			return nil, errors.Errorf("environment not empty, found %d service(s)", l)
		}
		return []txn.Op{{
			C:      environmentsC,
			Id:     st.EnvironUUID(),
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
