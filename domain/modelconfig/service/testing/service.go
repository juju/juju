// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/juju/domain/modeldefaults"
)

type modelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

func (m modelDefaultsProviderFunc) ModelDefaults(c context.Context) (modeldefaults.Defaults, error) {
	return m(c)
}

// ModelDefaultsProvider is a testing func that returns a
// service.ModelDefaultsProvider statically returning the defaults supplied to
// this func with no errors. It is also safe to pass a nil defaults to this func.
func ModelDefaultsProvider(defaults modeldefaults.Defaults) modelDefaultsProviderFunc {
	return func(_ context.Context) (modeldefaults.Defaults, error) {
		return defaults, nil
	}
}
