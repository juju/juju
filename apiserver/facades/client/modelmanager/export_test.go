// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"
)

func AuthCheck(c *gc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	mm.authCheck(user)
	return mm.isAdmin
}
