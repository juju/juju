package state

// Environment represents the state of an environment.
type Environment struct {
	st   *State
	name string
	annotator
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	conf, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env := &Environment{
		st:   st,
		name: conf.Name(),
	}
	env.annotator = annotator{
		globalKey: env.globalKey(),
		tag:       env.Tag(),
		st:        st,
	}
	return env, nil
}

// Tag returns a name identifying the environment.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (e Environment) Tag() string {
	return "environment-" + e.name
}

// globalKey returns the global database key for the environment.
func (e *Environment) globalKey() string {
	return "e#" + e.name
}
