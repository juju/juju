// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testhelpers"
)

type mockRelation struct {
	testhelpers.Stub
	id     int
	status status.StatusInfo
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id: id,
	}
}

func (r *mockRelation) Endpoints() []relation.Endpoint {
	r.MethodCall(r, "Endpoints")
	return nil
}

func (r *mockRelation) WatchUnits(applicationName string) (relation.RelationUnitsWatcher, error) {
	r.MethodCall(r, "WatchUnits")
	return nil, nil
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) SetStatus(info status.StatusInfo) error {
	r.status = info
	return nil
}
