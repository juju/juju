// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
}

func (w *mockWatcher) Stop() error {
	w.MethodCall(w, "Stop")
	if err := w.NextErr(); err != nil {
		return err
	}
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

func (w *mockWatcher) Err() error {
	w.MethodCall(w, "Err")
	return w.Tomb.Err()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRelation struct {
	testing.Stub
	firewall.Relation
	id      int
	ruw     *mockRelationUnitsWatcher
	inScope set.Strings
	status  status.StatusInfo
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:      id,
		ruw:     newMockRelationUnitsWatcher(),
		inScope: make(set.Strings),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) WatchRelationIngressNetworks() state.StringsWatcher {
	w := newMockStringsWatcher()
	w.changes <- []string{"1.2.3.4/32"}
	return w
}

func (r *mockRelation) SetStatus(info status.StatusInfo) error {
	r.status = info
	return nil
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{changes: make(chan params.RelationUnitsChange, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan params.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.changes
}
