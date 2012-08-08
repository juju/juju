package uniter

import (
	"errors"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
)

// HookState stores a hook's execution status.
type HookState struct {
	path string
}

// Running persists the fact that the Uniter is committed to running
// the supplied hook.
func (s *HookState) Started(hi HookInfo) error {
	return writeDiskHook(s.path, hi, "started")
}

// Succeeded persists the fact that the Uniter has successfully completed
// the supplied hook, but that it has not yet reconciled all the state it
// needs to.
func (s *HookState) Succeeded(hi HookInfo) error {
	return writeDiskHook(s.path, hi, "succeeded")
}

// Committed causes the HookState to entirely forget the supplied hook.
func (s *HookState) Committed(hi HookInfo) error {
	return os.Remove(s.path)
}

// NotSucceeded returns any hook that has been started but has not succeeded.
// If there is no such hook, it will return NoHook.
func (s *HookState) NotSucceeded() (HookInfo, error) {
	return readDiskHook(s.path, "started")
}

// NotCommitted returns any hook that has succeeded but not been committed.
// If there is no such hook, it will return NoHook.
func (s *HookState) NotCommitted() (HookInfo, error) {
	return readDiskHook(s.path, "succeeded")
}

// NoHook indicates that no hook exists with the requested characteristics.
var NoHook = errors.New("no hook")

// diskHook holds the hook information that needs to persist across runs of
// the unit agent process.
type diskHook struct {
	RelationId    int
	HookKind      string
	RemoteUnit    string
	ChangeVersion int
	Members       []string
	Status        string
}

// writeDiskHook atomically writes the supplied hook information and status.
func writeDiskHook(path string, hi HookInfo, status string) error {
	dh := diskHook{
		RelationId:    hi.RelationId,
		HookKind:      hi.HookKind,
		RemoteUnit:    hi.RemoteUnit,
		ChangeVersion: hi.ChangeVersion,
		Status:        status,
	}
	for m := range hi.Members {
		dh.Members = append(dh.Members, m)
	}
	return atomicWrite(path, dh)
}

// readDiskHook returns a HookInfo describing a hook persisted with the
// supplied status. If no such hook exists, NoHook is returned.
func readDiskHook(path, requireStatus string) (HookInfo, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = NoHook
		}
		return HookInfo{}, err
	}
	var dh diskHook
	if err = goyaml.Unmarshal(data, &dh); err != nil {
		return HookInfo{}, err
	}
	if dh.Status == requireStatus {
		return dh.HookInfo(), nil
	}
	return HookInfo{}, NoHook
}

// HookInfo returns a HookInfo reconstructed from the data stored in dh.
func (dh diskHook) HookInfo() HookInfo {
	hi := HookInfo{
		RelationId:    dh.RelationId,
		HookKind:      dh.HookKind,
		RemoteUnit:    dh.RemoteUnit,
		ChangeVersion: dh.ChangeVersion,
		Members:       map[string]map[string]interface{}{},
	}
	for _, m := range dh.Members {
		hi.Members[m] = nil
	}
	return hi
}
