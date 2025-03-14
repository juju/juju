// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/state"
)

// backend wraps a *state.State to implement Backend.
// It is untested, but is simple enough to be verified by inspection.
type backend struct {
	*state.State
	allRemoteApplications func() ([]commoncrossmodel.RemoteApplication, error)
}

func newBacked(st *state.State) Backend {
	return &backend{
		State:                 st,
		allRemoteApplications: commoncrossmodel.GetBackend(st).AllRemoteApplications}
}

// AllLocalRelatedModels returns all models on this controller to which
// another hosted model has a consuming cross model relation.
func (s *backend) AllLocalRelatedModels() ([]string, error) {
	uuids, err := s.AllModelUUIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	localUUIDs := set.NewStrings(uuids...)
	apps, err := s.allRemoteApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var crossModelUUIDs []string
	for _, app := range apps {
		if app.IsConsumerProxy() {
			continue
		}
		if localUUIDs.Contains(app.SourceModel().Id()) {
			crossModelUUIDs = append(crossModelUUIDs, app.SourceModel().Id())
		}
	}
	return crossModelUUIDs, nil
}
