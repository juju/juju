// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"sync"

	"github.com/juju/juju/internal/scriptlet"
)

var globalEnv = sync.OnceValue(func() *scriptlet.EnvRegistry {
	env := &scriptlet.EnvRegistry{}
	scriptlet.RegisterEnv(env)
	return env
})
