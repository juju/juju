// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"strings"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
)

// gocheck boilerplate.
type initialisationInternalSuite struct{}

var _ = gc.Suite(initialisationInternalSuite{})

func (initialisationInternalSuite) TestCreatePool(c *gc.C) {
	pathfinder := func(s string) (string, error) {
		return "/a-path", nil
	}
	stub := runStub{}
	err := createPool(pathfinder, stub.run)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		"virsh pool-define-as juju-kvm dir - - - - /a-path/guests",
		"virsh pool-build juju-kvm",
		"virsh pool-start juju-kvm",
		"virsh pool-autostart juju-kvm",
	})
}

type runStub struct {
	calls []string
}

func (s *runStub) run(cmd string, args ...string) (string, error) {
	call := []string{cmd}
	call = append(call, args...)
	s.calls = append(s.calls, strings.Join(call, " "))
	return "", nil
}

func (s *runStub) Calls() []string {
	return s.calls
}
