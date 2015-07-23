// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
)

// IPAddress represents an IP address as seen by an addresser
// worker.
type IPAddress struct {
	facade base.FacadeCaller
	tag    names.IPAddressTag
	life   params.Life
}

// Tag returns the IP address's tag.
func (a *IPAddress) Tag() names.IPAddressTag {
	return a.tag
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
