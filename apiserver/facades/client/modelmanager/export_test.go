// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
)

func AuthCheck(c *tc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	err := mm.authCheck(c.Context(), user)
	c.Assert(err, tc.ErrorIsNil)
	return mm.isAdmin
}
