package params

// UnitStatus represents the status of the unit or its agent.
type UnitStatus string

const (
	// The unit is not yet participating in the environment.
	UnitPending UnitStatus = "pending"

	// The unit has performed initial setup and is adapting itself to
	// the environment.
	UnitInstalled UnitStatus = "installed"

	// The unit is actively participating in the environment.
	UnitStarted UnitStatus = "started"

	// The unit's agent will perform no further action, other than to
	// set the unit to Dead at a suitable moment.
	UnitStopped UnitStatus = "stopped"

	// The unit requires human intervention in order to operate
	// correctly.
	UnitError UnitStatus = "error"

	// The unit agent ought to be signalling activity, but it cannot
	// be detected.
	UnitDown UnitStatus = "down"
)

// MachineStatus represents the status of the machine or its agent.
type MachineStatus string

const (
	// The machine is not yet participating in the environment.
	MachinePending MachineStatus = "pending"

	// The machine is actively participating in the environment.
	MachineStarted MachineStatus = "started"

	// The machine's agent will perform no further action, other than
	// to set the machine to Dead at a suitable moment.
	MachineStopped MachineStatus = "stopped"

	// The machine requires human intervention in order to operate
	// correctly.
	MachineError MachineStatus = "error"

	// The machine agent ought to be signalling activity, but it cannot
	// be detected.
	MachineDown MachineStatus = "down"
)
