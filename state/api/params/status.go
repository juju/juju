package params

// UnitStatus represents the status of the unit agent.
type UnitStatus string

const (
	UnitPending   UnitStatus = "pending"   // Agent hasn't started
	UnitInstalled UnitStatus = "installed" // Agent has run the installed hook
	UnitStarted   UnitStatus = "started"   // Agent is running properly
	UnitStopped   UnitStatus = "stopped"   // Agent has stopped running on request
	UnitError     UnitStatus = "error"     // Agent is waiting in an error state
	UnitDown      UnitStatus = "down"      // Agent is down or not communicating
)

// MachineStatus represents the status of the machine agent.
type MachineStatus string

const (
	MachinePending MachineStatus = "pending" // Agent hasn't started
	MachineStarted MachineStatus = "started" // Agent is running properly
	MachineStopped MachineStatus = "stopped" // Agent has stopped running on request
	MachineError   MachineStatus = "error"   // Agent is waiting in an error state
	MachineDown    MachineStatus = "down"    // Agent is down or not communicating
)
