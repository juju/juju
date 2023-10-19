// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"github.com/juju/juju/api/base"
	commonsecretdrain "github.com/juju/juju/api/common/secretsdrain"
)

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *commonsecretdrain.Client {
	return commonsecretdrain.NewClient(base.NewFacadeCaller(caller, "SecretsDrain"))
}
