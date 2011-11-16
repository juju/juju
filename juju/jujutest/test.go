package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
)

// Tests defines methods which test juju functionality against
// a given environment within Environs.
// Name gives the name of the environment.
type Tests struct {
	Environs *juju.Environs
	Name     string

	environs []juju.Environ
}

func (t *Tests) addEnviron(e juju.Environ) {
	t.environs = append(t.environs, e)
}

func (t *Tests) TearDownTest(c *C) {
	for _, e := range t.environs {
		err := e.Destroy()
		if err != nil {
			c.Errorf("error destroying environment after test: %v", err)
		}
	}
	t.environs = nil
}
