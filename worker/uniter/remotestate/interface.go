package remotestate

type Watcher interface {
	// RemoteStateChanged returns a channel which is signalled
	// whenever the remote state is changed.
	RemoteStateChanged() <-chan struct{}

	// Snapshot returns the current snapshot of the remote state.
	Snapshot() Snapshot

	Stop() error
	Wait() error
}
