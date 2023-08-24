// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

func init() {
	provider.Register(juju.NewProvider())
	provider.Register(kubernetes.NewProvider())
	provider.Register(vault.NewProvider())
}
