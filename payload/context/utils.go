// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
)

type componentHookFunction func() (Component, error)

func componentHookContext(ctx HookContext) componentHookFunction {
	return func() (Component, error) {
		compCtx, err := ContextComponent(ctx)
		if err != nil {
			// The component wasn't tracked properly.
			return nil, errors.Trace(err)
		}
		return compCtx, nil
	}
}
