package uniter

import "github.com/juju/juju/worker/uniter/hook"

// Storage encapsulates storage instance state and operations.
type Storage interface {

	// Hooks returns the channel on which storage hook execution requests
	// are sent.
	Hooks() <-chan hook.Info

	// StartHooks starts sending hook execution requests on the Hooks channel.
	StartHooks()

	// StopHooks stops sending hook execution requests on the Hooks channel.
	StopHooks() error

	// PrepareHook returns the name of the supplied storage hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hookInfo hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied storage
	// hook, or returns an error if the hook is unknown or invalid given
	// current state.
	CommitHook(hookInfo hook.Info) error

	// Update checks for and responds to changes to the storage instances
	// with the supplied ids.
	Update(ids []string) error

	// SetDying indicates that the only hooks to be requested should be
	// those necessary to cleanly destroy the storage instances.
	SetDying() error
}
