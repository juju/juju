package juju

// Unexpose hides a service from the internet.
func (c *Conn) Unxpose(service string) error {
	st, err := c.State()
	if err != nil {
		return err
	}
	svc, err := st.Service(service)
	if err != nil {
		return err
	}
	return svc.SetExposed()
}
