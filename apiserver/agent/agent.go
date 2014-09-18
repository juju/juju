// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machine package implements the API interfaces
// used by the machine agent.
package agent

import (
	"github.com/juju/juju/apiserver/common"
)

func init() {
	common.RegisterStandardFacade("Agent", 0, NewAgentAPIV0)
	common.RegisterStandardFacade("Agent", 1, NewAgentAPIV1)
}
