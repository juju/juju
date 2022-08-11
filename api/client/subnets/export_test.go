// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import "github.com/juju/juju/api/base"

func NewAPIFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
