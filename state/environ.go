package state

import "fmt"

// Environment represents the state of an environment.
type Environment struct {
	st *State
	annotator
}

// GetEnvironment returns the environment entity.
func (st *State) GetEnvironment() *Environment {
	env := &Environment{
		st:        st,
		annotator: annotator{st: st},
	}
	env.annotator.entityName = env.EntityName()
	return env
}

// EntityName returns a name identifying the environment.
// The returned name will be different from other EntityName values returned
// by any other entities from the same state.
func (e Environment) EntityName() string {
	return "environment"
}

// SetPassword currently just returns an error. Implemented here so that
// an environment can be used as an Entity.
func (e Environment) SetPassword(pass string) error {
	return fmt.Errorf("cannot set password of environment")
}

// PasswordValid currently just returns false. Implemented here so that
// an environment can be used as an Entity.
func (e Environment) PasswordValid(pass string) bool {
	return false
}

// Refresh currently just returns an error. Implemented here so that
// an environment can be used as an Entity.
func (e Environment) Refresh() error {
	return fmt.Errorf("cannot refresh the environment")
}
