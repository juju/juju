package params

// Status represents the status of an entity.
// It could be a unit, machine or its agent.
type Status string

const (
	// The entity is not yet participating in the environment.
	StatusPending Status = "pending"

	// The unit has performed initial setup and is adapting itself to
	// the environment. Not applicable to machines.
	StatusInstalled Status = "installed"

	// The entity is actively participating in the environment.
	StatusStarted Status = "started"

	// The entity's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The entity requires human intervention in order to operate
	// correctly.
	StatusError Status = "error"

	// The entity ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)
