// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"

	"github.com/juju/errors"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/utils"
	uhook "launchpad.net/juju-core/worker/uniter/hook"
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

	// Continue indicates that the uniter should run ModeContinue
	// to determine the next operation.
	Continue Op = "continue"
)

// OpStep describes the recorded progression of an operation.
type OpStep string

const (
	// Queued indicates that the uniter should undertake the operation
	// as soon as possible.
	Queued OpStep = "queued"

	// Pending indicates that the uniter has started, but not completed,
	// the operation.
	Pending OpStep = "pending"

	// Done indicates that the uniter has completed the operation,
	// but may not yet have synchronized all necessary secondary state.
	Done OpStep = "done"
)

// State defines the local persistent state of the uniter, excluding relation
// state.
type State struct {
	// Started indicates whether the start hook has run.
	Started bool

	// Op indicates the current operation.
	Op Op

	// OpStep indicates the current operation's progression.
	OpStep OpStep

	// Hook holds hook information relevant to the current operation. If Op
	// is Continue, it holds the last hook that was executed; if Op is RunHook,
	// it holds the running hook; if Op is Upgrade, a non-nil hook indicates
	// that the uniter should return to that hook's Pending state after the
	// upgrade is complete (instead of running an upgrade-charm hook).
	Hook *uhook.Info `yaml:"hook,omitempty"`

	// Charm describes the charm being deployed by an Install or Upgrade
	// operation, and is otherwise blank.
	CharmURL *charm.URL `yaml:"charm,omitempty"`
}

// validate returns an error if the state violates expectations.
func (st State) validate() (err error) {
	defer errors.Maskf(&err, "invalid uniter state")
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
	case Continue, RunHook:
		if !hasHook {
			return fmt.Errorf("missing hook info")
		} else if hasCharm {
			return fmt.Errorf("unexpected charm URL")
		}
	default:
		return fmt.Errorf("unknown operation %q", st.Op)
	}
	switch st.OpStep {
	case Queued, Pending, Done:
	default:
		return fmt.Errorf("unknown operation step %q", st.OpStep)
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
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoStateFile
		}
	}
	if err := st.validate(); err != nil {
		return nil, fmt.Errorf("cannot read charm state at %q: %v", f.path, err)
	}
	return &st, nil
}

// Write stores the supplied state to the file.
func (f *StateFile) Write(started bool, op Op, step OpStep, hi *uhook.Info, url *charm.URL) error {
	st := &State{
		Started:  started,
		Op:       op,
		OpStep:   step,
		Hook:     hi,
		CharmURL: url,
	}
	if err := st.validate(); err != nil {
		panic(err)
	}
	return utils.WriteYaml(f.path, st)
}
