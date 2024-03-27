// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type mockCloudSpecAPI struct {
	// TODO - implement when remaining firewaller tests become unit tests
	cloudspec.CloudSpecAPI
}

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

func (r *mockRelation) SetStatus(info status.StatusInfo, recorder status.StatusHistoryRecorder) error {
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

type mockMachine struct {
	testing.Stub
	firewall.Machine

	id               string
	openedPortRanges *mockMachinePortRanges
	isManual         bool
}

func newMockMachine(id string) *mockMachine {
	return &mockMachine{
		id: id,
	}
}

func (st *mockMachine) Id() string {
	st.MethodCall(st, "Id")
	return st.id
}

func (st *mockMachine) IsManual() (bool, error) {
	st.MethodCall(st, "IsManual")
	if err := st.NextErr(); err != nil {
		return false, err
	}
	return st.isManual, nil
}

func (st *mockMachine) OpenedPortRanges() (state.MachinePortRanges, error) {
	st.MethodCall(st, "OpenedPortRanges")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	if st.openedPortRanges == nil {
		return nil, errors.NotFoundf("opened port ranges for machine %q", st.id)
	}
	return st.openedPortRanges, nil
}

type mockMachinePortRanges struct {
	state.MachinePortRanges

	byUnit map[string]*mockUnitPortRanges
}

func newMockMachinePortRanges(unitRanges ...*mockUnitPortRanges) *mockMachinePortRanges {
	byUnit := make(map[string]*mockUnitPortRanges)
	for _, upr := range unitRanges {
		byUnit[upr.unitName] = upr
	}

	return &mockMachinePortRanges{
		byUnit: byUnit,
	}
}

func (st *mockMachinePortRanges) ByUnit() map[string]state.UnitPortRanges {
	out := make(map[string]state.UnitPortRanges)
	for k, v := range st.byUnit {
		out[k] = v
	}

	return out
}

type mockUnitPortRanges struct {
	state.UnitPortRanges

	unitName   string
	byEndpoint network.GroupedPortRanges
}

func newMockUnitPortRanges(unitName string, byEndpoint network.GroupedPortRanges) *mockUnitPortRanges {
	return &mockUnitPortRanges{
		unitName:   unitName,
		byEndpoint: byEndpoint,
	}
}

func (st *mockUnitPortRanges) ByEndpoint() network.GroupedPortRanges {
	return st.byEndpoint
}
