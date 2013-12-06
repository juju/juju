// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs"
)

var (
	ShortAttempt = &shortAttempt
	APIVersion   = apiVersion
)

func MAASAgentName(env environs.Environ) string {
	return env.(*maasEnviron).ecfg().maasAgentName()
}

func GetMAASClient(env environs.Environ) *gomaasapi.MAASObject {
	return env.(*maasEnviron).getMAASClient()
}
