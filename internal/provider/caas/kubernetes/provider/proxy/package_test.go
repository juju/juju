// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"testing"

	gc "gopkg.in/check.v1"
	"k8s.io/client-go/rest"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (p *Proxier) RESTConfig() rest.Config {
	return p.restConfig
}

func (p *Proxier) Config() ProxierConfig {
	return p.config
}
