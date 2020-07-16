// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"os"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/loggo"
)

type ConnectFunc func(agent.Agent) (api.Connection, error)

func DefaultConnect(a agent.Agent) (api.Connection, error) {
	return apicaller.OnlyConnect(a, api.Open, loggo.GetLogger("juju.agent"))
}

type ConfigFunc func() agent.Config

func DefaultConfig() agent.Config {
	return &configFromEnv{}
}

type IdentityFunc func() Identity

func DefaultIdentity() Identity {
	return Identity{
		PodName: os.Getenv("JUJU_K8S_POD_NAME"),
		PodUUID: os.Getenv("JUJU_K8S_POD_UUID"),
	}
}

type Identity struct {
	PodName string
	PodUUID string
}
