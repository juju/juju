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
	env.annotator = annotator{env.globalKey(), env.EntityName(), st}
	return env, nil
}

// EntityName returns a name identifying the environment.
// The returned name will be different from other EntityName values returned
// by any other entities from the same state.
func (e Environment) EntityName() string {
	return "environment-" + e.name
}

// globalKey returns the global database key for the environment.
func (e *Environment) globalKey() string {
	return "e#" + e.name
}
