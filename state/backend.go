// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/state/watcher"
)

// modelBackend collects together some useful internal state methods for
// accessing mongo and mapping local and global ids to one another.
type modelBackend interface {
	ModelUUID() string
	IsController() bool

	// docID generates a globally unique ID value
	// where the model UUID is prefixed to the
	// localID.
	docID(string) string

	// localID returns the local ID value by stripping
	// off the model UUID prefix if it is there.
	localID(string) string

	// strictLocalID returns the local ID value by removing the
	// model UUID prefix. If there is no prefix matching the
	// State's model, an error is returned.
	strictLocalID(string) (string, error)

	// nowToTheSecond returns the current time in UTC to the nearest second. We use
	// this for a time source that is not more precise than we can handle. When
	// serializing time in and out of mongo, we lose enough precision that it's
	// misleading to store any more than precision to the second.
	nowToTheSecond() time.Time

	clock() clock.Clock
	db() Database
	modelName() (string, error)
	txnLogWatcher() watcher.BaseWatcher
}

func (st *State) docID(localID string) string {
	return ensureModelUUID(st.ModelUUID(), localID)
}

func (st *State) localID(id string) string {
	modelUUID, localID, ok := splitDocID(id)
	if !ok || modelUUID != st.ModelUUID() {
		return id
	}
	return localID
}

func (st *State) strictLocalID(id string) (string, error) {
	modelUUID, localID, ok := splitDocID(id)
	if !ok || modelUUID != st.ModelUUID() {
		return "", errors.Errorf("unexpected id: %#v", id)
	}
	return localID, nil
}

func (st *State) clock() clock.Clock {
	return st.stateClock
}

func (st *State) modelName() (string, error) {
	m, err := st.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.NameOld(), nil
}

func (st *State) nowToTheSecond() time.Time {
	return st.clock().Now().Round(time.Second).UTC()
}
