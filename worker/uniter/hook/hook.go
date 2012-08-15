package hook

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
)

// Kind enumerates the different kinds of hooks implemented by charms.
type Kind string

const (
	// None of these hooks are ever associated with a relation; each of them
	// represents a change to the state of the unit as a whole. The values
	// themselves are all valid hook names.
	Install       Kind = "install"
	Start         Kind = "start"
	ConfigChanged Kind = "config-changed"
	UpgradeCharm  Kind = "upgrade-charm"

	// These hooks require an associated relation, and the name of the relation
	// unit whose change triggered the hook. The values are not themselves valid
	// hook names: they need to be prefixed with the name of the associated
	// relation, and a hyphen, before they can be executed.
	RelationJoined   Kind = "relation-joined"
	RelationChanged  Kind = "relation-changed"
	RelationDeparted Kind = "relation-departed"

	// This hook requires an associated relation. To get a valid hook name from
	// the value, if must be prefixed just like the other Relation* Kind values.
	RelationBroken Kind = "relation-broken"
)

// Valid will return true if the Kind is known.
func (kind Kind) Valid() bool {
	switch kind {
	case Install, Start, ConfigChanged, UpgradeCharm:
	case RelationJoined, RelationChanged, RelationDeparted:
	case RelationBroken:
	default:
		return false
	}
	return true
}

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind Kind

	// RelationId identifies the relation associated with the hook. It is
	// only set when Kind indicates a relation hook.
	RelationId int

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind inicates a relation hook other than relation-broken.
	RemoteUnit string

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int

	// Members contains the latest known state of the relation; its set of
	// keys is the set of unit names that should be treated as present in
	// the relation. The values may contain up-to-date relation settings
	// for the member units, but these are communicated only when already
	// known to the producer: their presence should never be assumed. The
	// field is only set when Kind identifies a relation hook.
	Members map[string]map[string]interface{}
}

// Status defines the stages of execution through which a hook passes.
type Status string

const (
	// StatusStarted indicates that the unit agent intended to run the hook.
	// This status implies that a hook *may* have been interrupted and have
	// failed to complete all required operations, and that therefore the
	// proper response is to treat it as a hook execution failure and punt
	// to the user for manual resolution.
	StatusStarted Status = "started"

	// StatusSucceeded indicates that the hook itself completed successfully,
	// but that local state (ie relation membership) may not have been
	// synchronized, and that recovery should therefore be performed.
	StatusSucceeded Status = "succeeded"

	// StatusCommitted indicates that the last hook ran successfully and that
	// local state has been synchronized.
	StatusCommitted Status = "committed"
)

// Valid will return true if the Status is known.
func (status Status) Valid() bool {
	switch status {
	case StatusStarted, StatusSucceeded, StatusCommitted:
		return true
	}
	return false
}

// state defines the hook state serialization.
type state struct {
	RelationId    int
	Kind          Kind
	RemoteUnit    string
	ChangeVersion int
	Members       []string
	Status        Status
}

// StateFile stores and retrieves a hook and its execution status.
type StateFile struct {
	path string
}

// NewStateFile returns a new hook state that persists to the supplied path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

// ErrNoStateFile indicates that no hook has ever been stored.
var ErrNoStateFile = errors.New("hook state file does not exist")

// Read reads the current hook state from disk. It returns ErrNoStateFile if
// the file doesn't exist.
func (f *StateFile) Read() (info Info, status Status, err error) {
	var data []byte
	if data, err = ioutil.ReadFile(f.path); err != nil {
		if os.IsNotExist(err) {
			err = ErrNoStateFile
		}
		return
	}
	var st state
	if err = goyaml.Unmarshal(data, &st); err != nil {
		return
	}
	if !st.Kind.Valid() || !st.Status.Valid() {
		err = fmt.Errorf("invalid hook state at %s", f.path)
		return
	}
	info = Info{
		Kind:          st.Kind,
		RelationId:    st.RelationId,
		RemoteUnit:    st.RemoteUnit,
		ChangeVersion: st.ChangeVersion,
		Members:       map[string]map[string]interface{}{},
	}
	for _, m := range st.Members {
		info.Members[m] = nil
	}
	status = st.Status
	return
}

// Write writes the supplied hook state to disk. It panics if asked to store
// invalid data.
func (f *StateFile) Write(info Info, status Status) error {
	if !status.Valid() {
		panic(fmt.Errorf("unknown hook status %q", status))
	}
	if !info.Kind.Valid() {
		panic(fmt.Errorf("unknown hook kind %q", info.Kind))
	}
	st := state{
		Kind:          info.Kind,
		RelationId:    info.RelationId,
		RemoteUnit:    info.RemoteUnit,
		ChangeVersion: info.ChangeVersion,
		Status:        status,
	}
	for m := range info.Members {
		st.Members = append(st.Members, m)
	}
	return atomicWrite(f.path, &st)
}

// atomicWrite marshals obj as yaml and then writes it to a file atomically
// by first writing a sibling with the suffix ".preparing", and then moving
// the sibling to the real path.
func atomicWrite(path string, obj interface{}) error {
	data, err := goyaml.Marshal(obj)
	if err != nil {
		return err
	}
	preparing := ".preparing"
	if err = ioutil.WriteFile(path+preparing, data, 0644); err != nil {
		return err
	}
	return os.Rename(path+preparing, path)
}
