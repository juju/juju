// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	names_v3 "gopkg.in/juju/names.v3"

	"github.com/juju/juju/state/watcher"
)

//go:generate mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/state/watcher BaseWatcher

// modelBackend collects together some useful internal state methods for
// accessing mongo and mapping local and global ids to one another.
type modelBackend interface {
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
	modelUUID() string
	modelName() (string, error)
	isController() bool
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

// ParseLocalIDToTags tries to parse a DocID e.g. `c9741ea1-0c2a-444d-82f5-787583a48557:a#mediawiki
// to a corresponding Tag. In the above case -> applicationTag.
func (st *State) ParseLocalIDToTags(docID string) names_v3.Tag {
	_, localID, _ := splitDocID(docID)
	switch {
	case strings.HasPrefix(localID, "a#"):
		return names_v3.NewApplicationTag(localID[2:])
	case strings.HasPrefix(localID, "m#"):
		return names_v3.NewMachineTag(localID)
	case strings.HasPrefix(localID, "u#"):
		return names_v3.NewUnitTag(localID[2:])
	case strings.HasPrefix(localID, "e"):
		return names_v3.NewModelTag(localID)
	default:
		return nil
	}
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

func (st *State) modelUUID() string {
	return st.ModelUUID()
}

func (st *State) modelName() (string, error) {
	m, err := st.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.Name(), nil
}

func (st *State) isController() bool {
	return st.IsController()
}

func (st *State) nowToTheSecond() time.Time {
	return st.clock().Now().Round(time.Second).UTC()
}
