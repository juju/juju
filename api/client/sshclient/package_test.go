// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/juju/api/base"
)

func NewFacadeFromCaller(caller base.FacadeCaller) *Facade {
	return &Facade{
		caller: caller,
	}
}
