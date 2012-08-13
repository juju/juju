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
	StatusStarted   HookStatus = "started"
	StatusSucceeded HookStatus = "succeeded"
	StatusCommitted HookStatus = "committed"
)

// validate will return an error if the HookStatus is not a known value.
func (hs HookStatus) validate() (err error) {
	switch hs {
	case StatusStarted, StatusSucceeded, StatusCommitted:
	default:
		err = fmt.Errorf("unknown HookStatus %q!", hs)
	}
	return
}

// diskHook defines the hook state serialization.
type diskHook struct {
	RelationId    int
	HookKind      string
	RemoteUnit    string
	ChangeVersion int
	Members       []string
	Status        HookStatus
}

// HookState stores and retrieves a hook and its execution status.
type HookState struct {
	path string
}

// NewHookState returns a new hook state that persists to the supplied path.
func NewHookState(path string) *HookState {
	return &HookState{path}
}

// ErrNoHook indicates that no hook has ever been stored.
var ErrNoHook = errors.New("no hook")

// Get loads the status of, and the HookInfo, that was last Set. If no hook
// has ever been set, the error will be ErrNoHook.
func (s *HookState) Get() (hi HookInfo, hs HookStatus, err error) {
	var data []byte
	if data, err = ioutil.ReadFile(s.path); err != nil {
		if os.IsNotExist(err) {
			err = ErrNoHook
		}
		return
	}
	var dh diskHook
	if err = goyaml.Unmarshal(data, &dh); err != nil {
		return
	}
	if dh.HookKind == "" || dh.Status.validate() != nil {
		err = fmt.Errorf("invalid hook state at %s", s.path)
	} else {
		hi = HookInfo{
			RelationId:    dh.RelationId,
			HookKind:      dh.HookKind,
			RemoteUnit:    dh.RemoteUnit,
			ChangeVersion: dh.ChangeVersion,
			Members:       map[string]map[string]interface{}{},
		}
		for _, m := range dh.Members {
			hi.Members[m] = nil
		}
		hs = dh.Status
	}
	return
}

// Set persists the status of, and information necessary to reconstruct, the
// supplied hook. It will panic if asked to store invalid data.
func (s *HookState) Set(hi HookInfo, hs HookStatus) error {
	if err := hs.validate(); err != nil {
		panic(err)
	}
	if hi.HookKind == "" {
		panic("empty HookKind!")
	}
	dh := diskHook{
		RelationId:    hi.RelationId,
		HookKind:      hi.HookKind,
		RemoteUnit:    hi.RemoteUnit,
		ChangeVersion: hi.ChangeVersion,
		Status:        hs,
	}
	for m := range hi.Members {
		dh.Members = append(dh.Members, m)
	}
	return atomicWrite(s.path, dh)
}
