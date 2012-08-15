package uniter

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
)

// HookStatus defines the stages of execution through which a hook passes.
type HookStatus string

const (
	// StatusStarted indicates that the unit agent intended to run the hook.
	// This status implies that a hook *may* have been interrupted and have
	// failed to complete all required operations, and that therefore the
	// proper response is to treat it as a hook execution failure and punt
	// to the user for manual resolution.
	StatusStarted HookStatus = "started"

	// StatusSucceeded indicates that the hook itself completed successfully,
	// but that local state (ie relation membership) may not have been
	// synchronized, and that recovery may therefore be necessary.
	StatusSucceeded HookStatus = "succeeded"

	// StatusCommitted indicates that the last hook ran successfully and that
	// local state has been synchronized.
	StatusCommitted HookStatus = "committed"
)

// valid will return true if the value is known.
func (hs HookStatus) valid() bool {
	switch hs {
	case StatusStarted, StatusSucceeded, StatusCommitted:
		return true
	}
	return false
}

// hookState defines the hook state serialization.
type hookState struct {
	RelationId    int
	HookKind      string
	RemoteUnit    string
	ChangeVersion int
	Members       []string
	Status        HookStatus
}

// HookStateFile stores and retrieves a hook and its execution status.
type HookStateFile struct {
	path string
}

// NewHookStateFile returns a new hook state that persists to the supplied path.
func NewHookStateFile(path string) *HookStateFile {
	return &HookStateFile{path}
}

// ErrNoHookState indicates that no hook has ever been stored.
var ErrNoHookState = errors.New("no hook")

// Read reads the current hook state from disk. It returns ErrNoHookState if
// the file doesn't exist.
func (f *HookStateFile) Read() (hi HookInfo, hs HookStatus, err error) {
	var data []byte
	if data, err = ioutil.ReadFile(f.path); err != nil {
		if os.IsNotExist(err) {
			err = ErrNoHookState
		}
		return
	}
	var state hookState
	if err = goyaml.Unmarshal(data, &state); err != nil {
		return
	}
	if state.HookKind == "" || !state.Status.valid() {
		err = fmt.Errorf("invalid hook state at %s", f.path)
		return
	}
	hi = HookInfo{
		RelationId:    state.RelationId,
		HookKind:      state.HookKind,
		RemoteUnit:    state.RemoteUnit,
		ChangeVersion: state.ChangeVersion,
		Members:       map[string]map[string]interface{}{},
	}
	for _, m := range state.Members {
		hi.Members[m] = nil
	}
	hs = state.Status
	return
}

// Write writes the supplied hook state to disk. It panics if asked to store
// invalid data.
func (f *HookStateFile) Write(hi HookInfo, hs HookStatus) error {
	if !hs.valid() {
		panic(fmt.Errorf("unknown hook status %q", hs))
	}
	if hi.HookKind == "" {
		panic("empty HookKind!")
	}
	state := hookState{
		RelationId:    hi.RelationId,
		HookKind:      hi.HookKind,
		RemoteUnit:    hi.RemoteUnit,
		ChangeVersion: hi.ChangeVersion,
		Status:        hs,
	}
	for m := range hi.Members {
		state.Members = append(state.Members, m)
	}
	return atomicWrite(f.path, &state)
}
