// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"k8s.io/client-go/rest"
)


func (p *Proxier) RESTConfig() rest.Config {
	return p.restConfig
}

func (p *Proxier) Config() ProxierConfig {
	return p.config
}
