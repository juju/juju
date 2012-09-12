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
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind inicates a relation hook other than relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int `yaml:"change-version,omitempty"`

	// Members may contain settings for units that are members of the relation,
	// keyed on unit name. If a unit is present in members, it is always a
	// member of the relation; if a unit is not present, no inferences about
	// its state can be drawn.
	Members map[string]map[string]interface{} `yaml:"members,omitempty"`
}

// Status defines the stages of execution through which a hook passes.
type Status string

const (
	// Queued indicates that the hook should be executed at the earliest
	// opportunity.
	Queued Status = "queued"

	// Pending indicates that execution of the hook is pending. A hook
	// that fails should keep this status until it is successfully re-
	// executed or skipped.
	Pending Status = "pending"

	// Committing indicates that execution of the hook has successfully
	// completed, or that the hook has been skipped, but that persistent
	// local state has not been synchronized with the change embodied by
	// the hook.
	Committing Status = "committing"

	// Complete indicates that all operations associated with the hook have
	// succeeded.
	Complete Status = "complete"
)

// valid will return true if the Status is known.
func (status Status) valid() bool {
	switch status {
	case Queued, Pending, Committing, Complete:
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
	return State{
		Info: Info{
			Kind:          st.Kind,
			RelationId:    st.RelationId,
			RemoteUnit:    st.RemoteUnit,
			ChangeVersion: st.ChangeVersion,
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
	return trivial.WriteYaml(f.path, &st)
}

// state defines the hook state serialization.
type state struct {
	Kind          Kind
	RelationId    int    `yaml:"relation-id,omitempty"`
	RemoteUnit    string `yaml:"remote-unit,omitempty"`
	ChangeVersion int    `yaml:"change-version,omitempty"`
	Status        Status
}
