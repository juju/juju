// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	mgoutils "github.com/juju/juju/mongo/utils"
)

// unitStateDoc records the state persisted by the charm executing in the unit.
type unitStateDoc struct {
	// DocID is always the same as a unit's global key.
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	// The following maps to UnitState:

	// State encodes the unit's persisted charm state as a list of key-value pairs.
	CharmState map[string]string `bson:"charm-state,omitempty"`

	// UniterState is a serialized yaml string containing the uniters internal
	// state for this unit.
	UniterState string `bson:"uniter-state,omitempty"`

	// RelationState is a serialized yaml string containing relation internal
	// state for this unit from the uniter.
	RelationState map[string]string `bson:"relation-state,omitempty"`

	// StorageState is a serialized yaml string containing storage internal
	// state for this unit from the uniter.
	StorageState string `bson:"storage-state,omitempty"`

	// MeterStatusState is a serialized yaml string containing the internal
	// state for this unit's meter status worker.
	MeterStatusState string `bson:"meter-status-state,omitempty"`
}

// charmStateMatches returns true if the State map within the unitStateDoc matches
// the provided st argument.
func (d *unitStateDoc) charmStateMatches(st bson.M) bool {
	if len(st) != len(d.CharmState) {
		return false
	}

	for k, v := range d.CharmState {
		if st[k] != v {
			return false
		}
	}

	return true
}

// relationData translate the unitStateDoc's RelationState as
// a map[string]string to a map[int]string, as is needed.
// BSON does not allow ints as a map key.
func (d *unitStateDoc) relationData() (map[int]string, error) {
	if d.RelationState == nil {
		return nil, nil
	}
	// BSON maps cannot have an int as key.
	rState := make(map[int]string, len(d.RelationState))
	for k, v := range d.RelationState {
		kString, err := strconv.Atoi(k)
		if err != nil {
			return nil, err
		}
		rState[kString] = v
	}
	return rState, nil
}

// relationStateMatches returns true if the RelationState map within the
// unitStateDoc contains all of the provided newRS map.  Assumes that
// relationStateBSONFriendly has been called first.
func (d *unitStateDoc) relationStateMatches(newRS map[string]string) bool {
	if len(d.RelationState) != len(newRS) {
		return false
	}
	for k, v := range newRS {
		if d.RelationState[k] != v {
			return false
		}
	}
	return true
}

// removeUnitStateOp returns the operation needed to remove the unit state
// document associated with the given globalKey.
func removeUnitStateOp(mb modelBackend, globalKey string) txn.Op {
	return txn.Op{
		C:      unitStatesC,
		Id:     mb.docID(globalKey),
		Remove: true,
	}
}

// UnitState contains the various state saved for this unit,
// including from the charm itself and the uniter.
type UnitState struct {
	// charmState encodes the unit's persisted charm state as a list of
	// key-value pairs.
	charmState    map[string]string
	charmStateSet bool

	// uniterState is a serialized yaml string containing the uniters internal
	// state for this unit.
	uniterState    string
	uniterStateSet bool

	// relationState is a serialized yaml string containing relation internal
	// state for this unit from the uniter.
	relationState    map[int]string
	relationStateSet bool

	// storageState is a serialized yaml string containing storage internal
	// state for this unit from the uniter.
	storageState    string
	storageStateSet bool

	// meterStatusState is a serialized yaml string containing the internal
	// state for the meter status worker for this unit.
	meterStatusState    string
	meterStatusStateSet bool
}

// NewUnitState returns a new UnitState struct.
func NewUnitState() *UnitState {
	return &UnitState{}
}

// Modified returns true if any of the struct have been set.
func (u *UnitState) Modified() bool {
	return u.relationStateSet ||
		u.storageStateSet ||
		u.charmStateSet ||
		u.uniterStateSet ||
		u.meterStatusStateSet
}

// SetCharmState sets the charm state value.
func (u *UnitState) SetCharmState(state map[string]string) {
	u.charmStateSet = true
	u.charmState = state
}

// CharmState returns the unit's stored charm state and bool indicating
// whether the data was set.
func (u *UnitState) CharmState() (map[string]string, bool) {
	return u.charmState, u.charmStateSet
}

// SetUniterState sets the uniter state value.
func (u *UnitState) SetUniterState(state string) {
	u.uniterStateSet = true
	u.uniterState = state
}

// UniterState returns the uniter state and bool indicating
// whether the data was set.
func (u *UnitState) UniterState() (string, bool) {
	return u.uniterState, u.uniterStateSet
}

// SetRelationState sets the relation state value.
func (u *UnitState) SetRelationState(state map[int]string) {
	u.relationStateSet = true
	u.relationState = state
}

// RelationState returns the relation state and bool indicating
// whether the data was set.
func (u *UnitState) RelationState() (map[int]string, bool) {
	return u.relationState, u.relationStateSet
}

// relationStateBSONFriendly makes a map[int]string BSON friendly by
// translating the int map key to a string.
func (u *UnitState) relationStateBSONFriendly() (map[string]string, bool) {
	stringData := make(map[string]string, len(u.relationState))
	for k, v := range u.relationState {
		stringData[strconv.Itoa(k)] = v
	}
	return stringData, u.relationStateSet
}

// SetStorageState sets the storage state value.
func (u *UnitState) SetStorageState(state string) {
	u.storageStateSet = true
	u.storageState = state
}

// StorageState returns the storage state and bool indicating
// whether the data was set.
func (u *UnitState) StorageState() (string, bool) {
	return u.storageState, u.storageStateSet
}

// SetMeterStatusState sets the state value for meter state.
func (u *UnitState) SetMeterStatusState(state string) {
	u.meterStatusStateSet = true
	u.meterStatusState = state
}

// MeterStatusState returns the meter status state and a bool to indicate
// whether the data was set.
func (u *UnitState) MeterStatusState() (string, bool) {
	return u.meterStatusState, u.meterStatusStateSet
}

// SetState replaces the currently stored state for a unit with the contents
// of the provided UnitState.
//
// Use this for testing, otherwise use SetStateOperation.
func (u *Unit) SetState(unitState *UnitState, limits UnitStateSizeLimits) error {
	modelOp := u.SetStateOperation(unitState, limits)
	return u.st.ApplyOperation(modelOp)
}

// SetStateOperation returns a ModelOperation for replacing the currently
// stored state for a unit with the contents of the provided UnitState.
func (u *Unit) SetStateOperation(unitState *UnitState, limits UnitStateSizeLimits) ModelOperation {
	return &unitSetStateOperation{u: u, newState: unitState, limits: limits}
}

// State returns the persisted state for a unit.
func (u *Unit) State() (*UnitState, error) {
	us := NewUnitState()

	// Normally this would be if Life() != Alive.  However the uniter
	// needs to read its state during the Dying period for the case of
	// hook failures just before dying.  It's unclear whether this is
	// an actual scenario or just part of the UniterSuite unit tests.
	// See TestUniterDyingReaction.
	if u.Life() == Dead {
		return us, errors.NotFoundf("unit %s", u.Name())
	}

	coll, closer := u.st.db().GetCollection(unitStatesC)
	defer closer()

	var stDoc unitStateDoc
	if err := coll.FindId(u.globalKey()).One(&stDoc); err != nil {
		if err == mgo.ErrNotFound {
			return us, nil
		}
		return us, errors.Trace(err)
	}

	if stDoc.RelationState != nil {
		rState, err := stDoc.relationData()
		if err != nil {
			return us, errors.Trace(err)
		}
		us.SetRelationState(rState)
	}

	if stDoc.CharmState != nil {
		charmState := make(map[string]string, len(stDoc.CharmState))
		for k, v := range stDoc.CharmState {
			charmState[mgoutils.UnescapeKey(k)] = v
		}
		us.SetCharmState(charmState)
	}

	us.SetUniterState(stDoc.UniterState)
	us.SetStorageState(stDoc.StorageState)
	us.SetMeterStatusState(stDoc.MeterStatusState)

	return us, nil
}

// UnitStateSizeLimits defines the quota limits that are enforced when updating
// the state (charm and uniter) of a unit.
type UnitStateSizeLimits struct {
	// The maximum allowed size for the charm state. It can be set to zero
	// to bypass the charm state quota checks.
	// quota checks will be
	MaxCharmStateSize int

	// The maximum allowed size for the uniter's state. It can be set to
	// zero to bypass the uniter state quota checks.
	MaxAgentStateSize int
}
