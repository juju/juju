// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

func AuthCheck(c *tc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	err := mm.authCheck(context.Background(), user)
	c.Assert(err, jc.ErrorIsNil)
	return mm.isAdmin
}
