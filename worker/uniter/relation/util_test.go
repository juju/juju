// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker/uniter/hook"
)

func hookList(kinds ...hooks.Kind) []hook.Info {
	result := make([]hook.Info, len(kinds))
	for i, kind := range kinds {
		result[i].Kind = kind
	}
	return result
}

type updateSource struct {
	tomb    tomb.Tomb
	empty   bool
	changes chan multiwatcher.RelationUnitsChange
	updates chan multiwatcher.RelationUnitsChange
}

func newEmptySource() *updateSource {
	return newUpdateSource(true, false)
}

func newFullBufferedSource() *updateSource {
	return newUpdateSource(false, true)
}

func newFullUnbufferedSource() *updateSource {
	return newUpdateSource(false, false)
}

func newUpdateSource(empty, buffered bool) *updateSource {
	var bufferSize int
	if buffered {
		bufferSize = 1000
	}
	source := &updateSource{
		empty:   empty,
		changes: make(chan multiwatcher.RelationUnitsChange),
		updates: make(chan multiwatcher.RelationUnitsChange, bufferSize),
	}
	go func() {
		defer source.tomb.Done()
		defer close(source.changes)
		<-source.tomb.Dying()
	}()
	return source
}

func (source *updateSource) Stop() error {
	source.tomb.Kill(nil)
	return source.tomb.Wait()
}

func (source *updateSource) Changes() <-chan multiwatcher.RelationUnitsChange {
	return source.changes
}

func (source *updateSource) Update(change multiwatcher.RelationUnitsChange) error {
	select {
	case <-source.tomb.Dying():
		return tomb.ErrDying
	case source.updates <- change:
	}
	return nil
}

func (source *updateSource) Empty() bool {
	return source.empty
}

func (source *updateSource) Next() hook.Info {
	if source.empty {
		panic(nil)
	}
	return hook.Info{Kind: hooks.Install}
}

func (source *updateSource) Pop() {
	if source.empty {
		panic(nil)
	}
}
