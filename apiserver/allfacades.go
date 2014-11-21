// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

// This file imports all of the facades so they get registered at runtime.
// When adding a new facade implementation, import it here so that its init()
// function will get called to register it.
import (
	_ "github.com/juju/juju/apiserver/action"
	_ "github.com/juju/juju/apiserver/agent"
	_ "github.com/juju/juju/apiserver/backups"
	_ "github.com/juju/juju/apiserver/charmrevisionupdater"
	_ "github.com/juju/juju/apiserver/client"
	_ "github.com/juju/juju/apiserver/deployer"
	_ "github.com/juju/juju/apiserver/diskformatter"
	_ "github.com/juju/juju/apiserver/diskmanager"
	_ "github.com/juju/juju/apiserver/environment"
	_ "github.com/juju/juju/apiserver/firewaller"
	_ "github.com/juju/juju/apiserver/keymanager"
	_ "github.com/juju/juju/apiserver/keyupdater"
	_ "github.com/juju/juju/apiserver/logger"
	_ "github.com/juju/juju/apiserver/machine"
	_ "github.com/juju/juju/apiserver/metricsmanager"
	_ "github.com/juju/juju/apiserver/networker"
	_ "github.com/juju/juju/apiserver/provisioner"
	_ "github.com/juju/juju/apiserver/reboot"
	_ "github.com/juju/juju/apiserver/rsyslog"
	_ "github.com/juju/juju/apiserver/service"
	_ "github.com/juju/juju/apiserver/uniter"
	_ "github.com/juju/juju/apiserver/upgrader"
	_ "github.com/juju/juju/apiserver/usermanager"
)
