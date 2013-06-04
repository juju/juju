// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
	"time"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	listener net.Listener
}

var _ = Suite(&suite{})

func chanReadEmpty(c *C, ch <-chan struct{}, what string) bool {
	select {
	case _, ok := <-ch:
		return ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func chanReadStrings(c *C, ch <-chan []string, what string) ([]string, bool) {
	select {
	case changes, ok := <-ch:
		return changes, ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func chanReadConfig(c *C, ch <-chan *config.Config, what string) (*config.Config, bool) {
	select {
	case envConfig, ok := <-ch:
		return envConfig, ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func removeServiceAndUnits(c *C, service *state.Service) {
	// Destroy all units for the service.
	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	for _, unit := range units {
		err = unit.EnsureDead()
		c.Assert(err, IsNil)
		err = unit.Remove()
		c.Assert(err, IsNil)
	}
	err = service.Destroy()
	c.Assert(err, IsNil)

	err = service.Refresh()
	c.Assert(errors.IsNotFoundError(err), Equals, true)
}

// apiAuthenticator represents a simple authenticator object with only the
// SetPassword and Tag methods.  This will fit types from both the state
// and api packages, as those in the api package do not have PasswordValid().
type apiAuthenticator interface {
	state.Tagger
	SetPassword(string) error
}

func setDefaultPassword(c *C, e apiAuthenticator) {
	err := e.SetPassword(defaultPassword(e))
	c.Assert(err, IsNil)
}

func defaultPassword(e apiAuthenticator) string {
	return e.Tag() + " password"
}

type setStatuser interface {
	SetStatus(status params.Status, info string) error
}

func setDefaultStatus(c *C, entity setStatuser) {
	err := entity.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
}

func (s *suite) tryOpenState(c *C, e apiAuthenticator, password string) error {
	stateInfo := s.StateInfo(c)
	stateInfo.Tag = e.Tag()
	stateInfo.Password = password
	st, err := state.Open(stateInfo, state.DialOpts{
		Timeout:    25 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	return err
}

// openAs connects to the API state as the given entity
// with the default password for that entity.
func (s *suite) openAs(c *C, tag string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Tag = tag
	info.Password = fmt.Sprintf("%s password", tag)
	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	return st
}
