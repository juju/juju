// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hooktesting

import (
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/uniter/hook"
)

func HookList(kinds ...hooks.Kind) []hook.Info {
	result := make([]hook.Info, len(kinds))
	for i, kind := range kinds {
		result[i].Kind = kind
	}
	return result
}

type UpdateSource struct {
	Tomb     tomb.Tomb
	empty    bool
	ChangesC chan hook.SourceChange
	UpdatesC chan interface{}
}

func NewEmptySource() *UpdateSource {
	return newUpdateSource(true, false)
}

func NewFullBufferedSource() *UpdateSource {
	return newUpdateSource(false, true)
}

func NewFullUnbufferedSource() *UpdateSource {
	return newUpdateSource(false, false)
}

func newUpdateSource(empty, buffered bool) *UpdateSource {
	var bufferSize int
	if buffered {
		bufferSize = 1000
	}
	source := &UpdateSource{
		empty:    empty,
		ChangesC: make(chan hook.SourceChange),
		UpdatesC: make(chan interface{}, bufferSize),
	}
	go func() {
		defer source.Tomb.Done()
		defer close(source.ChangesC)
		<-source.Tomb.Dying()
	}()
	return source
}

func (source *UpdateSource) Stop() error {
	source.Tomb.Kill(nil)
	return source.Tomb.Wait()
}

func (source *UpdateSource) Changes() <-chan hook.SourceChange {
	return source.ChangesC
}

func (source *UpdateSource) NewChange(v interface{}) hook.SourceChange {
	return func() error {
		select {
		case <-source.Tomb.Dying():
			return tomb.ErrDying
		case source.UpdatesC <- v:
		}
		return nil
	}
}

func (source *UpdateSource) Empty() bool {
	return source.empty
}

func (source *UpdateSource) Next() hook.Info {
	if source.empty {
		panic(nil)
	}
	return hook.Info{Kind: hooks.Install}
}

func (source *UpdateSource) Pop() {
	if source.empty {
		panic(nil)
	}
}
