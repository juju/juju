// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// IPAddress represents an IP address as seen by an addresser
// worker.
type IPAddress struct {
	facade base.FacadeCaller

	tag  names.IPAddressTag
	life params.Life
}

// Id returns the IP address's id.
func (a *IPAddress) Id() string {
	return a.tag.Id()
}

// Tag returns the IP address's tag.
func (a *IPAddress) Tag() names.IPAddressTag {
	return a.tag
}

// String returns the IP address as a string.
func (a *IPAddress) String() string {
	return a.Id()
}

// Life returns the IP address's lifecycle value.
func (a *IPAddress) Life() params.Life {
	return a.life
}

// Refresh updates the cached local copy of the IP address's data.
func (a *IPAddress) Refresh() error {
	life, err := common.Life(a.facade, a.tag)
	if err != nil {
		return errors.Trace(err)
	}
	a.life = life
	return nil
}

// Remove removes the IP address.
func (a *IPAddress) Remove() error {
	life, err := common.Life(a.facade, a.tag)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
