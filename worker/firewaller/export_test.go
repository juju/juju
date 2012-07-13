package firewaller

// AllMachines returns the ids of all monitored machines.
func (fw *Firewaller) AllMachines() []int {
	all := []int{}
	for _, machine := range fw.machines {
		all = append(all, machine.id)
	}
	return all
}

// CloseState allows to close the state of the firewaller
// externally.
func (fw *Firewaller) CloseState() error {
	return fw.st.Close()
}
