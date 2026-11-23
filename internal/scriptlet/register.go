// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

// RegisterEnv registers the common scriptlet declarations.
func RegisterEnv(env *EnvRegistry) {
	env.AddAppMethod(setStatusBuiltin)
}
