package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter/hook"
	"os"
)

// Op enumerates the operations the uniter can perform.
type Op string

const (
	// Install indicates that the uniter is installing the charm.
	Install Op = "install"

	// RunHook indicates that the uniter is running a hook.
	RunHook Op = "run-hook"

	// Upgrade indicates that the uniter is upgrading the charm.
	Upgrade Op = "upgrade"

	// Abide indicates that the uniter is not currently executing
	// any other operation.
	Abide Op = "abide"
)

// Status enumerates the possible operation statuses.
type Status string

const (
	// Queued indicates that the uniter should undertake the operation
	// as soon as possible.
	Queued Status = "queued"

	// Pending indicates that the uniter has started, but not completed,
	// the operation.
	Pending Status = "pending"

	// Committing indicates that the uniter has completed the operation,
	// but has yet to synchronize all necessary state.
	Committing Status = "committing"
)

// State defines the local persistent state of the uniter, excluding relation
// state.
type State struct {
	// Op indicates the current operation.
	Op Op

	// Status indicates the current operation's status.
	Status Status

	// Hook holds hook information relevant to the current operation. If Op
	// is Abide, it holds the last hook that was executed; if Op is RunHook,
	// it holds the running hook; if Op is Upgrade, a non-nil hook indicates
	// that the uniter should return to that hook's Pending state after the
	// upgrade is complete (instead of running an upgrade-charm hook).
	Hook *hook.Info `yaml:"hook,omitempty"`

	// Charm describes the charm being deployed by an Install or Upgrade
	// operation, and is otherwise blank.
	CharmURL *charm.URL `yaml:"charm,omitempty"`
}

// validate returns an error if the state violates expectations.
func (st State) validate() error {
	hasHook := st.Hook != nil
	hasCharm := st.CharmURL != nil
	switch st.Op {
	case Install:
		if hasHook {
			return fmt.Errorf("unexpected hook info")
		}
		fallthrough
	case Upgrade:
		if !hasCharm {
			return fmt.Errorf("missing charm URL")
		}
	case Abide, RunHook:
		if !hasHook {
			return fmt.Errorf("missing hook info")
		} else if hasCharm {
			return fmt.Errorf("unexpected charm URL")
		}
	default:
		return fmt.Errorf("unknown operation %q", st.Op)
	}
	switch st.Status {
	case Queued, Pending, Committing:
	default:
		return fmt.Errorf("unknown operation status %q", st.Status)
	}
	if hasHook {
		return st.Hook.Validate()
	}
	return nil
}

// StateFile holds the disk state for a uniter.
type StateFile struct {
	path string
}

// NewStateFile returns a new StateFile using path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

var ErrNoStateFile = errors.New("uniter state file does not exist")

// Read reads a State from the file. If the file does not exist it returns
// ErrNoStateFile.
func (f *StateFile) Read() (*State, error) {
	var st State
	if err := trivial.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoStateFile
		}
	}
	if err := st.validate(); err != nil {
		return nil, fmt.Errorf("invalid uniter state at %q: %v", f.path, err)
	}
	return &st, nil
}

// Write stores the supplied state to the file.
func (f *StateFile) Write(op Op, status Status, hi *hook.Info, url *charm.URL) error {
	if hi != nil {
		// Strip membership info: it's potentially large, and can
		// be reconstructed from relation state when required.
		hiCopy := *hi
		hiCopy.Members = nil
		hi = &hiCopy
	}
	st := &State{
		Op:       op,
		Status:   status,
		Hook:     hi,
		CharmURL: url,
	}
	if err := st.validate(); err != nil {
		panic(err)
	}
	return trivial.WriteYaml(f.path, st)
}
