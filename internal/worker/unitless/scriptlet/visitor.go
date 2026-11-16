// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import "github.com/canonical/starlark/starlark"

// AppObjectMethods returns the "juju" AppObject builtins exposed to
// scriptlets.
func AppObjectMethods() []*starlark.Builtin {
	return []*starlark.Builtin{
		// TODO (manadart 2026-07-06): There will be two builtin types.
		// 1) Those that append intents (like this one), which will be
		//    reusable in agents and probably live in a scriptlet domain.
		// 2) Those that query external state, and will *not* be common.
		//    This is because agents will need to go via an API, and
		//    server-side workers directly via domain services.
		// I forsee a visitor defined in the domain, which will add all
		// the intent builtins to a script set.
		setStatusBuiltin,
	}
}
