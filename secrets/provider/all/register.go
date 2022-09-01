// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
)

func init() {
	provider.Register(juju.Store, juju.NewProvider())
	provider.Register(kubernetes.Store, kubernetes.NewProvider())
}
