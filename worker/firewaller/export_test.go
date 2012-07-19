package firewaller

// CloseState allows to close the state of the firewaller
// externally.
func (fw *Firewaller) CloseState() error {
	return fw.st.Close()
}
