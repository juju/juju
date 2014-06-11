// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

// This file just imports all of the facades so they get registered at runtime
import (
	_ "github.com/juju/juju/state/apiserver/agent"
	_ "github.com/juju/juju/state/apiserver/charmrevisionupdater"
	_ "github.com/juju/juju/state/apiserver/client"
	_ "github.com/juju/juju/state/apiserver/deployer"
	_ "github.com/juju/juju/state/apiserver/environment"
	_ "github.com/juju/juju/state/apiserver/firewaller"
	_ "github.com/juju/juju/state/apiserver/keymanager"
	_ "github.com/juju/juju/state/apiserver/keyupdater"
	_ "github.com/juju/juju/state/apiserver/logger"
	_ "github.com/juju/juju/state/apiserver/machine"
	_ "github.com/juju/juju/state/apiserver/provisioner"
	_ "github.com/juju/juju/state/apiserver/rsyslog"
	_ "github.com/juju/juju/state/apiserver/uniter"
	_ "github.com/juju/juju/state/apiserver/upgrader"
	_ "github.com/juju/juju/state/apiserver/usermanager"
)
