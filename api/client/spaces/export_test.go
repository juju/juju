// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/juju/v3/api/base"
)

func NewAPIFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
