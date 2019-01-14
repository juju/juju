// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"strconv"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

type relationUnitsSettingsFunc func([]string) ([]params.SettingsResult, error)

// relationUnitsWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type relationUnitsWorker struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	ruw         watcher.RelationUnitsWatcher
	changes     chan<- params.RemoteRelationChangeEvent

	applicationToken    string
	macaroon            *macaroon.Macaroon
	remoteRelationToken string

	unitSettingsFunc relationUnitsSettingsFunc
}

func newRelationUnitsWorker(
	relationTag names.RelationTag,
	applicationToken string,
	macaroon *macaroon.Macaroon,
	remoteRelationToken string,
	ruw watcher.RelationUnitsWatcher,
	changes chan<- params.RemoteRelationChangeEvent,
	unitSettingsFunc relationUnitsSettingsFunc,
) (*relationUnitsWorker, error) {
	w := &relationUnitsWorker{
		relationTag:         relationTag,
		applicationToken:    applicationToken,
		macaroon:            macaroon,
		remoteRelationToken: remoteRelationToken,
		ruw:                 ruw,
		changes:             changes,
		unitSettingsFunc:    unitSettingsFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{ruw},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *relationUnitsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *relationUnitsWorker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		logger.Errorf("error in relation units worker for %v: %v", w.relationTag.Id(), err)
	}
	return err
}

func (w *relationUnitsWorker) loop() error {
	var (
		changes chan<- params.RemoteRelationChangeEvent
		event   params.RemoteRelationChangeEvent
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.ruw.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			logger.Debugf("relation units changed for %v: %#v", w.relationTag, change)
			if evt, err := w.relationUnitsChangeEvent(change); err != nil {
				return errors.Trace(err)
			} else {
				if evt == nil {
					continue
				}
				event = *evt
				changes = w.changes
			}
		case changes <- event:
			changes = nil
		}
	}
}

func (w *relationUnitsWorker) relationUnitsChangeEvent(
	change watcher.RelationUnitsChange,
) (*params.RemoteRelationChangeEvent, error) {
	logger.Debugf("update relation units for %v", w.relationTag)
	if len(change.Changed)+len(change.Departed) == 0 {
		return nil, nil
	}
	// Ensure all the changed units have been exported.
	changedUnitNames := make([]string, 0, len(change.Changed))
	for name := range change.Changed {
		changedUnitNames = append(changedUnitNames, name)
	}

	// unitNum parses a unit name and extracts the unit number.
	unitNum := func(unitName string) (int, error) {
		parts := strings.Split(unitName, "/")
		if len(parts) < 2 {
			return -1, errors.NotValidf("unit name %v", unitName)
		}
		return strconv.Atoi(parts[1])
	}

	// Construct the event to send to the remote model.
	event := &params.RemoteRelationChangeEvent{
		RelationToken:    w.remoteRelationToken,
		ApplicationToken: w.applicationToken,
		Macaroons:        macaroon.Slice{w.macaroon},
		DepartedUnits:    make([]int, len(change.Departed)),
	}
	for i, u := range change.Departed {
		num, err := unitNum(u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		event.DepartedUnits[i] = num
	}

	if len(changedUnitNames) > 0 {
		// For changed units, we publish/consume the current settings values.
		results, err := w.unitSettingsFunc(changedUnitNames)
		if err != nil {
			return nil, errors.Annotate(err, "fetching relation units settings")
		}
		for i, result := range results {
			if result.Error != nil {
				return nil, errors.Annotatef(result.Error, "fetching relation unit settings for %v", changedUnitNames[i])
			}
		}
		for i, result := range results {
			num, err := unitNum(changedUnitNames[i])
			if err != nil {
				return nil, errors.Trace(err)
			}
			change := params.RemoteRelationUnitChange{
				UnitId:   num,
				Settings: make(map[string]interface{}),
			}
			for k, v := range result.Settings {
				change.Settings[k] = v
			}
			event.ChangedUnits = append(event.ChangedUnits, change)
		}
	}
	return event, nil
}
