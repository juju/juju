// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	apiinstancemutater "github.com/juju/juju/v3/api/agent/instancemutater"
	"github.com/juju/juju/v3/api/base"
)

func NewClient(apiCaller base.APICaller) InstanceMutaterAPI {
	facade := apiinstancemutater.NewClient(apiCaller)
	return facade
}
