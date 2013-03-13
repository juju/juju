package state

// Environment represents the state of an environment.
type Environment struct {
	st *State
	annotator
	name string
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	conf, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env := &Environment{
		st:        st,
		annotator: annotator{st: st},
		name:      conf.Name(),
	}
	env.annotator.entityName = env.EntityName()
	return env, nil
}

// EntityName returns a name identifying the environment.
// The returned name will be different from other EntityName values returned
// by any other entities from the same state.
func (e Environment) EntityName() string {
	return "environment-" + e.name
}
