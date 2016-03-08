// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
)

const apiName = "ProxyUpdater"

// Facade provides access to a machine model worker's view of the world.
type Facade struct {
	*common.ModelWatcher
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{
		ModelWatcher: common.NewModelWatcher(facadeCaller),
	}
}

// TODO(wallyworld) - add methods for getting proxy settings specifically,
// rather than the entire model config.
// Also WatchProxySettings instead of WatchForModelConfigChanges.
