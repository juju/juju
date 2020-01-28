// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/juju/api/base"
)

func NewStateFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
