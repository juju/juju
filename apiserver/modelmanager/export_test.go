// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/names"
)

func AuthCheck(c *gc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	mm.authCheck(user)
	return mm.isAdmin
}

func NewModelManagerAPIForTest(
	st stateInterface,
	authorizer common.Authorizer,
	toolsFinder *common.ToolsFinder,
) *ModelManagerAPI {
	return &ModelManagerAPI{st, authorizer, toolsFinder, false}
}
