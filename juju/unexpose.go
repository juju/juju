package juju

// Expose exposes a service to the internet.
func (c *Conn) Expose(service string) error {
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
