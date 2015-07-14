// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"
)

// Service exposes the testing-required functionality of a service.
type Service interface {
	// SetConfig updates the service's config settings.
	SetConfig(c *gc.C, settings map[string]string)
	// Deploy adds a unit of the service to Juju.
	Deploy(c *gc.C) Unit
}

type service struct {
	env       *environ
	charmName string
	name      string
}

func newService(env *environ, charmName, serviceName string) *service {
	return &service{
		env:       env,
		charmName: charmName,
		name:      serviceName,
	}
}

// SetConfig implements Service.
func (svc *service) SetConfig(c *gc.C, settings map[string]string) {
	args := []string{
		svc.name,
	}
	for k, v := range settings {
		args = append(args, k+"="+v)
	}
	svc.env.run(c, "service set", args...)

	// TODO(ericsnow) Wait until done.
}

// Deploy implements Service.
func (svc *service) Deploy(c *gc.C) Unit {
	out := svc.env.run(c, "service add", svc.name)
	unit := newUnit(svc, out)

	// TODO(ericsnow) Wait until done.

	return unit
}
