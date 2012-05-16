package state

// support functions for watcher_test.go

func NewMachine(st *State, key string) *Machine {
	return &Machine{
		st:  st,
		key: key,
	}
}
