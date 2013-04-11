package params

// Status represents the status of the unit, machine or their agent.
type Status string

const (
	// The unit/machine is not yet participating in the environment.
	StatusPending Status = "pending"

	// The unit has performed initial setup and is adapting itself to
	// the environment. Not applicable to machines.
	StatusInstalled Status = "installed"

	// The unit/machine is actively participating in the environment.
	StatusStarted Status = "started"

	// The unit/machine's agent will perform no further action, other
	// than to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The unit/machine requires human intervention in order to
	// operate correctly.
	StatusError Status = "error"

	// The unit/machine agent ought to be signalling activity, but it
	// cannot be detected.
	StatusDown Status = "down"
)
