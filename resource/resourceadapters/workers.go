// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/resource/workers"
	"github.com/juju/juju/state"
)

// NewLatestCharmHandler returns a LatestCharmHandler that uses the
// given Juju state.
func NewLatestCharmHandler(st *state.State) (charmrevisionupdater.LatestCharmHandler, error) {
	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	handler := workers.NewLatestCharmHandler(resources)
	return handler, nil
}
