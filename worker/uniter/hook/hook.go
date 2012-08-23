// hook provides types and constants that define the hooks known to Juju,
// and implements persistence of hook execution state.
package hook

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/trivial"
	"os"
)

// Kind enumerates the different kinds of hooks that exist.
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
	// unit whose change triggered the hook. The hook file names that these
	// kinds represent will be prefixed by the relation name; for example,
	// "db-relation-joined".
	RelationJoined   Kind = "relation-joined"
	RelationChanged  Kind = "relation-changed"
	RelationDeparted Kind = "relation-departed"

	// This hook requires an associated relation. The represented hook file name
	// will be prefixed by the relation name, just like the other Relation* Kind
	// values.
	RelationBroken Kind = "relation-broken"
)

// valid will return true if the Kind is known.
func (kind Kind) valid() bool {
	switch kind {
	case Install, Start, ConfigChanged, UpgradeCharm:
	case RelationJoined, RelationChanged, RelationDeparted:
	case RelationBroken:
	default:
		return false
	}
	return true
}

// IsRelation will return true if the Kind represents a relation hook.
func (kind Kind) IsRelation() bool {
	switch kind {
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
	// Started indicates that the unit agent intended to run the hook.
	// This status implies that a hook *may* have been interrupted and have
	// failed to complete all required operations, and that therefore the
	// proper response is to treat it as a hook execution failure and punt
	// to the user for manual resolution.
	Started Status = "started"

	// Succeeded indicates that the hook itself completed successfully,
	// but that local state (ie relation membership) may not have been
	// synchronized, and that recovery should therefore be performed.
	Succeeded Status = "succeeded"

	// Committed indicates that the last hook ran successfully and that
	// local state has been synchronized.
	Committed Status = "committed"
)

// valid will return true if the Status is known.
func (status Status) valid() bool {
	switch status {
	case Started, Succeeded, Committed:
		return true
	}
	return false
}

// State holds details necessary for executing a hook, and the
// status of the execution.
type State struct {
	Info   Info
	Status Status
}

// StateFile stores and retrieves hook state.
type StateFile struct {
	path string
}

// NewStateFile returns a new state file that uses the supplied path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

// ErrNoStateFile indicates that no hook has ever been written.
var ErrNoStateFile = errors.New("hook state file does not exist")

// Read reads the current hook state from disk. It returns ErrNoStateFile if
// the file doesn't exist.
func (f *StateFile) Read() (State, error) {
	var st state
	if err := trivial.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return State{}, ErrNoStateFile
		}
		return State{}, err
	}
	if !st.Kind.valid() || !st.Status.valid() {
		return State{}, fmt.Errorf("invalid hook state at %s", f.path)
	}
	members := map[string]map[string]interface{}{}
	for _, m := range st.Members {
		members[m] = nil
	}
	return State{
		Info: Info{
			Kind:          st.Kind,
			RelationId:    st.RelationId,
			RemoteUnit:    st.RemoteUnit,
			ChangeVersion: st.ChangeVersion,
			Members:       members,
		},
		Status: st.Status,
	}, nil
}

// Write writes the supplied hook state to disk. It panics if asked to store
// invalid data.
func (f *StateFile) Write(info Info, status Status) error {
	if !status.valid() {
		panic(fmt.Errorf("unknown hook status %q", status))
	}
	if !info.Kind.valid() {
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
	return trivial.WriteYaml(f.path, &st)
}

// state defines the hook state serialization.
type state struct {
	Kind          Kind
	RelationId    int      `yaml:"relation-id,omitempty"`
	RemoteUnit    string   `yaml:"remote-unit,omitempty"`
	ChangeVersion int      `yaml:"change-version,omitempty"`
	Members       []string `yaml:"members,omitempty"`
	Status        Status
}
