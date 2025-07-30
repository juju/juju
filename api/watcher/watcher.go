// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/kr/pretty"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api/base"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.api.watcher")

const (
	// WatcherStopTimeout is the time to wait for a watcher to stop.
	WatcherStopTimeout = time.Second * 60
)

// commonWatcher implements common watcher logic in one place to
// reduce code duplication, but it's not in fact a complete watcher;
// it's intended for embedding.
type commonWatcher struct {
	tomb tomb.Tomb
	in   chan interface{}

	// These fields must be set by the embedding watcher, before
	// calling init().

	// newResult must return a pointer to a value of the type returned
	// by the watcher's Next call.
	newResult func() interface{}

	// call should invoke the given API method, placing the call's
	// returned value in result (if any).
	call watcherAPICall
}

// watcherAPICall wraps up the information about what facade and what watcher
// Id we are calling, and just gives us a simple way to call a common method
// with a given return value.
type watcherAPICall func(ctx context.Context, method string, result interface{}) error

// makeWatcherAPICaller creates a watcherAPICall function for a given facade name
// and watcherId.
func makeWatcherAPICaller(caller base.APICaller, facadeName, watcherId string) watcherAPICall {
	bestVersion := caller.BestFacadeVersion(facadeName)
	return func(ctx context.Context, request string, result interface{}) error {
		return caller.APICall(ctx, facadeName, bestVersion,
			watcherId, request, nil, &result)
	}
}

// init must be called to initialize an embedded commonWatcher's
// fields. Make sure newResult and call fields are set beforehand.
func (w *commonWatcher) init() {
	w.in = make(chan interface{})
	if w.newResult == nil {
		panic("newResult must be set")
	}
	if w.call == nil {
		panic("call must be set")
	}
}

// commonLoop implements the loop structure common to the client
// watchers. It should be started in a separate goroutine by any
// watcher that embeds commonWatcher. It kills the commonWatcher's
// tomb when an error occurs.
func (w *commonWatcher) commonLoop() {
	defer close(w.in)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// When the watcher has been stopped, we send a Stop request
		// to the server, which will remove the watcher and return a
		// CodeStopped error to any currently outstanding call to
		// Next. If a call to Next happens just after the watcher has
		// been stopped, we'll get a CodeNotFound error; Either way
		// we'll return, wait for the stop request to complete, and
		// the watcher will die with all resources cleaned up.
		defer wg.Done()
		<-w.tomb.Dying()

		// Give a reasonable amount of time for the watcher to stop. If it
		// doesn't stop in time, log the error and return.
		ctx, cancel := context.WithTimeout(context.Background(), WatcherStopTimeout)
		defer cancel()

		// We need to stop the watcher before returning, so that the
		// server can clean up resources.
		err := w.call(ctx, "Stop", nil)
		if err == nil {
			return
		}

		// If the deadline is exceeded whilst attempting to stop the watcher,
		// give up and log the error as debug. This is indicative of a bad
		// watcher, one that is not responding to the stop request.
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Debugf(context.TODO(), "timeout stopping watcher: %v", err)
			return
		}

		// Don't log an error if a watcher is stopped due to an agent restart,
		// or if the entity being watched is already removed.
		if !isAgentRestartError(err) &&
			err.Error() != rpc.ErrShutdown.Error() && !params.IsCodeNotFound(err) {
			logger.Errorf(context.TODO(), "error trying to stop watcher: %v", err)
			return
		}
	}()

	ctx, cancel := context.WithCancel(w.tomb.Context(context.Background()))
	defer cancel()

	wg.Add(1)
	go func() {
		// Because Next blocks until there are changes, we need to
		// call it in a separate goroutine, so the watcher can be
		// stopped normally.
		defer wg.Done()
		for {
			result := w.newResult()
			err := w.call(ctx, "Next", &result)
			if err != nil {
				// If the call was canceled, stopped or not found, and the
				// watcher is not alive, report it as a dying error.
				if params.IsCodeStopped(err) || params.IsCodeNotFound(err) || errors.Is(err, context.Canceled) {
					if w.tomb.Err() != tomb.ErrStillAlive {
						// The watcher has been stopped at the client end, so we're
						// expecting one of the above two kinds of error.
						// We might see the same errors if the server itself
						// has been shut down, in which case we leave them
						// untouched.
						err = tomb.ErrDying
					}
				}
				// Something went wrong, just report the error and bail out.
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.in <- result:
				// Report back the result we just got.
			}
		}
	}()
	wg.Wait()
}

var (
	// ErrRestartArgent matches juju/juju/internal/worker/error.go ErrRestartAgent
	// and is used to indicate that the watcher should be restarted.
	ErrRestartAgent = errors.New("agent should be restarted")
)

func isAgentRestartError(err error) bool {
	return err.Error() == ErrRestartAgent.Error()
}

// Kill is part of the worker.Worker interface.
func (w *commonWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *commonWatcher) Wait() error {
	return w.tomb.Wait()
}

// notifyWatcher will send events when something changes.
// It does not send content for those changes.
type notifyWatcher struct {
	commonWatcher
	caller          base.APICaller
	notifyWatcherId string
	out             chan struct{}
}

// If an API call returns a NotifyWatchResult, you can use this to turn it into
// a local Watcher.
func NewNotifyWatcher(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
	w := &notifyWatcher{
		caller:          caller,
		notifyWatcherId: result.NotifyWatcherId,
		out:             make(chan struct{}),
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *notifyWatcher) loop() error {
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = makeWatcherAPICaller(w.caller, "NotifyWatcher", w.notifyWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Since for a notifyWatcher there are no changes to send, we
		// just set the event (initial first, then after each change).
		case w.out <- struct{}{}:
		case <-w.tomb.Dying():
			return nil
		}
		if _, ok := <-w.in; !ok {
			// The tomb is already killed with the correct
			// error at this point, so just return.
			return nil
		}
	}
}

// Changes returns a channel that receives a value when a given entity
// changes in some way.
func (w *notifyWatcher) Changes() <-chan struct{} {
	return w.out
}

// stringsWatcher will send events when something changes.
// The content of the changes is a list of strings.
type stringsWatcher struct {
	commonWatcher
	caller           base.APICaller
	stringsWatcherId string
	out              chan []string
}

func NewStringsWatcher(caller base.APICaller, result params.StringsWatchResult) watcher.StringsWatcher {
	w := &stringsWatcher{
		caller:           caller,
		stringsWatcherId: result.StringsWatcherId,
		out:              make(chan []string),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

func (w *stringsWatcher) loop(initialChanges []string) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.StringsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "StringsWatcher", w.stringsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = data.(*params.StringsWatchResult).Changes
	}
}

// Changes returns a channel that receives a list of strings of watched
// entities with changes.
func (w *stringsWatcher) Changes() <-chan []string {
	return w.out
}

// relationUnitsWatcher will sends notifications of units entering and
// leaving the scope of a RelationUnit, and changes to the settings of
// those units known to have entered.
type relationUnitsWatcher struct {
	commonWatcher
	caller                 base.APICaller
	logger                 corelogger.Logger
	relationUnitsWatcherId string
	out                    chan watcher.RelationUnitsChange
}

func NewRelationUnitsWatcher(caller base.APICaller, result params.RelationUnitsWatchResult) watcher.RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		caller:                 caller,
		logger:                 logger.Child("relationunits"),
		relationUnitsWatcherId: result.RelationUnitsWatcherId,
		out:                    make(chan watcher.RelationUnitsChange),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

func copyRelationUnitsChanged(src params.RelationUnitsChange) watcher.RelationUnitsChange {
	dst := watcher.RelationUnitsChange{
		Departed: src.Departed,
	}
	if src.Changed != nil {
		dst.Changed = make(map[string]watcher.UnitSettings, len(src.Changed))
		for name, unitSettings := range src.Changed {
			dst.Changed[name] = watcher.UnitSettings{
				Version: unitSettings.Version,
			}
		}
	}
	if src.AppChanged != nil {
		dst.AppChanged = make(map[string]int64, len(src.AppChanged))
		for name, appVersion := range src.AppChanged {
			dst.AppChanged[name] = appVersion
		}
	}
	return dst
}

func (w *relationUnitsWatcher) loop(initialChanges params.RelationUnitsChange) error {
	changes := copyRelationUnitsChanged(initialChanges)
	w.newResult = func() interface{} { return new(params.RelationUnitsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "RelationUnitsWatcher", w.relationUnitsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
			if w.logger.IsLevelEnabled(corelogger.TRACE) {
				w.logger.Tracef(context.TODO(), "sent relation units changes %# v", pretty.Formatter(changes))
			}
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = copyRelationUnitsChanged(data.(*params.RelationUnitsWatchResult).Changes)
	}
}

// Changes returns a channel that will receive the changes to
// counterpart units in a relation. The first event on the channel
// holds the initial state of the relation in its Changed field.
func (w *relationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.out
}

// RemoteRelationWatcher is a worker that emits remote relation change
// events. It's not defined in core/watcher because it emits params
// structs - this makes more sense than converting to a core struct
// just to convert back when the event is published to the other
// model's API.
type RemoteRelationWatcher = watcher.Watcher[params.RemoteRelationChangeEvent]

// remoteRelationWatcher sends notifications of units entering and
// leaving scope of a relation and changes to unit/application
// settings, but yielding fleshed-out events so the caller doesn't
// need to call back to get the settings values.
type remoteRelationWatcher struct {
	commonWatcher
	caller                  base.APICaller
	logger                  corelogger.Logger
	remoteRelationWatcherId string
	out                     chan params.RemoteRelationChangeEvent
}

// NewRemoteRelationWatcher returns a RemoteRelationWatcher receiving
// events from the one running on the API server.
func NewRemoteRelationWatcher(caller base.APICaller, result params.RemoteRelationWatchResult) RemoteRelationWatcher {
	w := &remoteRelationWatcher{
		caller:                  caller,
		logger:                  logger.Child("remoterelations"),
		remoteRelationWatcherId: result.RemoteRelationWatcherId,
		out:                     make(chan params.RemoteRelationChangeEvent),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

func (w *remoteRelationWatcher) loop(initialChange params.RemoteRelationChangeEvent) error {
	change := initialChange
	w.newResult = func() interface{} { return new(params.RemoteRelationWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "RemoteRelationWatcher", w.remoteRelationWatcherId)
	w.commonWatcher.init()
	w.tomb.Go(func() error {
		w.commonLoop()
		return nil
	})

	for {
		select {
		// Send out the initial event or subsequent change.
		case w.out <- change:
			if w.logger.IsLevelEnabled(corelogger.TRACE) {
				w.logger.Tracef(context.TODO(), "sent remote relation change %# v", pretty.Formatter(change))
			}
		case <-w.tomb.Dying():
			return nil
		}

		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error at
			// this point, so just return.
			return nil
		}
		result, ok := data.(*params.RemoteRelationWatchResult)
		if !ok {
			return errors.Errorf("expected *params.RemoteRelationWatchResult, got %#v", data)
		}
		if result.Error != nil {
			return errors.Trace(result.Error)
		}
		change = result.Changes
	}
}

// Changes returns a channel that will emit changes to the remote
// relation units.
func (w *remoteRelationWatcher) Changes() <-chan params.RemoteRelationChangeEvent {
	return w.out
}

// SettingsGetter is a callback function the remote relation
// compatibility watcher calls to get unit settings when expanding
// events.
type SettingsGetter func([]string) ([]params.SettingsResult, error)

// remoteRelationCompatWatcher is a compatibility adaptor that calls
// back to the v1 crossmodelrelations API methods to wrap a
// server-side RelationUnitsWatcher into a RemoteRelationWatcher. It
// needs the relation and application token to include in the outgoing
// change events.
type remoteRelationCompatWatcher struct {
	commonWatcher
	caller                 base.APICaller
	relationUnitsWatcherId string
	relationToken          string
	appToken               string
	getSettings            SettingsGetter
	out                    chan params.RemoteRelationChangeEvent
}

// NewRemoteRelationCompatWatcher returns a RemoteRelationWatcher
// based on a server-side RelationUnitsWatcher.
func NewRemoteRelationCompatWatcher(
	caller base.APICaller,
	result params.RelationUnitsWatchResult,
	relationToken string,
	appToken string,
	getSettings SettingsGetter,
) RemoteRelationWatcher {
	w := &remoteRelationCompatWatcher{
		caller:                 caller,
		relationUnitsWatcherId: result.RelationUnitsWatcherId,
		relationToken:          relationToken,
		appToken:               appToken,
		getSettings:            getSettings,
		out:                    make(chan params.RemoteRelationChangeEvent),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

func (w *remoteRelationCompatWatcher) loop(initialChange params.RelationUnitsChange) error {
	change := initialChange
	w.newResult = func() interface{} { return new(params.RelationUnitsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "RelationUnitsWatcher", w.relationUnitsWatcherId)
	w.commonWatcher.init()

	// Ensure the worker loop waits for the commonLoop to stop before
	// being fully finished.
	w.tomb.Go(func() error {
		w.commonLoop()
		return nil
	})

	for {
		expanded, err := w.expandChange(change)
		if err != nil {
			return errors.Trace(err)
		}
		select {
		// Send out the initial event or subsequent change.
		case w.out <- expanded:
		case <-w.tomb.Dying():
			return nil
		}
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error at
			// this point, so just return.
			return nil
		}
		result, ok := data.(*params.RelationUnitsWatchResult)
		if !ok {
			return errors.Errorf("expected *params.RelationUnitsWatchResult, got %#v", data)
		}
		if result.Error != nil {
			return errors.Trace(result.Error)
		}
		change = result.Changes
	}
}

func (w *remoteRelationCompatWatcher) expandChange(change params.RelationUnitsChange) (params.RemoteRelationChangeEvent, error) {
	var empty params.RemoteRelationChangeEvent
	var unitNames []string
	for unit := range change.Changed {
		unitNames = append(unitNames, unit)
	}
	var changedUnits []params.RemoteRelationUnitChange
	if len(unitNames) > 0 {
		results, err := w.getSettings(unitNames)
		if err != nil {
			return empty, errors.Annotatef(err, "getting relation unit settings for %q", unitNames)
		}
		for i, result := range results {
			num, err := names.UnitNumber(unitNames[i])
			if err != nil {
				return empty, errors.Trace(err)
			}
			unitChange := params.RemoteRelationUnitChange{
				UnitId:   num,
				Settings: make(map[string]interface{}),
			}
			for k, v := range result.Settings {
				unitChange.Settings[k] = v
			}
			changedUnits = append(changedUnits, unitChange)
		}
	}

	var departedUnits []int
	for _, unit := range change.Departed {
		num, err := names.UnitNumber(unit)
		if err != nil {
			return empty, errors.Trace(err)
		}
		departedUnits = append(departedUnits, num)
	}

	expanded := params.RemoteRelationChangeEvent{
		RelationToken:           w.relationToken,
		ApplicationOrOfferToken: w.appToken,
		ChangedUnits:            changedUnits,
		DepartedUnits:           departedUnits,
	}
	// No need to handle AppChanged here - the v1 API can't tell us
	// app settings.
	return expanded, nil

}

// Changes returns a channel that will emit changes to the remote
// relation units.
func (w *remoteRelationCompatWatcher) Changes() <-chan params.RemoteRelationChangeEvent {
	return w.out
}

// relationStatusWatcher will sends notifications of changes to
// relation life and suspended status.
type relationStatusWatcher struct {
	commonWatcher
	caller                  base.APICaller
	relationStatusWatcherId string
	out                     chan []watcher.RelationStatusChange
}

// NewRelationStatusWatcher returns a watcher notifying of changes to
// relation life and suspended status.
func NewRelationStatusWatcher(
	caller base.APICaller, result params.RelationLifeSuspendedStatusWatchResult,
) watcher.RelationStatusWatcher {
	w := &relationStatusWatcher{
		caller:                  caller,
		relationStatusWatcherId: result.RelationStatusWatcherId,
		out:                     make(chan []watcher.RelationStatusChange),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

// mergeChanges combines the status changes in current and new, such that we end up with
// only one change per offer in the result; the most recent change wins.
func (w *relationStatusWatcher) mergeChanges(current, new []watcher.RelationStatusChange) []watcher.RelationStatusChange {
	chMap := make(map[string]watcher.RelationStatusChange)
	for _, c := range current {
		chMap[c.Key] = c
	}
	for _, c := range new {
		chMap[c.Key] = c
	}
	var result []watcher.RelationStatusChange
	for _, c := range chMap {
		result = append(result, c)
	}
	return result
}

func (w *relationStatusWatcher) loop(initialChanges []params.RelationLifeSuspendedStatusChange) error {
	w.newResult = func() interface{} { return new(params.RelationLifeSuspendedStatusWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "RelationStatusWatcher", w.relationStatusWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	copyChanges := func(changes []params.RelationLifeSuspendedStatusChange) []watcher.RelationStatusChange {
		result := make([]watcher.RelationStatusChange, len(changes))
		for i, ch := range changes {
			result[i] = watcher.RelationStatusChange{
				Key:             ch.Key,
				Life:            ch.Life,
				Suspended:       ch.Suspended,
				SuspendedReason: ch.SuspendedReason,
			}
		}
		return result
	}
	out := w.out
	changes := copyChanges(initialChanges)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
			// Read the next change.
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			newChanges := copyChanges(data.(*params.RelationLifeSuspendedStatusWatchResult).Changes)
			changes = w.mergeChanges(changes, newChanges)
			out = w.out
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// Changes returns a channel that will receive the changes to
// the life and status of a relation. The first event reflects the current
// values of these attributes.
func (w *relationStatusWatcher) Changes() watcher.RelationStatusChannel {
	return w.out
}

// offerStatusWatcher will send notifications of changes to offer status.
type offerStatusWatcher struct {
	commonWatcher
	caller               base.APICaller
	offerStatusWatcherId string
	out                  chan []watcher.OfferStatusChange
}

// NewOfferStatusWatcher returns a watcher notifying of changes to
// offer status.
func NewOfferStatusWatcher(
	caller base.APICaller, result params.OfferStatusWatchResult,
) watcher.OfferStatusWatcher {
	w := &offerStatusWatcher{
		caller:               caller,
		offerStatusWatcherId: result.OfferStatusWatcherId,
		out:                  make(chan []watcher.OfferStatusChange),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

// mergeChanges combines the status changes in current and new, such that we end up with
// only one change per offer in the result; the most recent change wins.
func (w *offerStatusWatcher) mergeChanges(current, new []watcher.OfferStatusChange) []watcher.OfferStatusChange {
	chMap := make(map[string]watcher.OfferStatusChange)
	for _, c := range current {
		chMap[c.Name] = c
	}
	for _, c := range new {
		chMap[c.Name] = c
	}
	var result []watcher.OfferStatusChange
	for _, c := range chMap {
		result = append(result, c)
	}
	return result
}

func (w *offerStatusWatcher) loop(initialChanges []params.OfferStatusChange) error {
	w.newResult = func() interface{} { return new(params.OfferStatusWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "OfferStatusWatcher", w.offerStatusWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	copyChanges := func(changes []params.OfferStatusChange) []watcher.OfferStatusChange {
		result := make([]watcher.OfferStatusChange, len(changes))
		for i, ch := range changes {
			result[i] = watcher.OfferStatusChange{
				Name: ch.OfferName,
				Status: status.StatusInfo{
					Status:  ch.Status.Status,
					Message: ch.Status.Info,
					Data:    ch.Status.Data,
					Since:   ch.Status.Since,
				},
			}
		}
		return result
	}
	out := w.out
	changes := copyChanges(initialChanges)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		// Read the next change.
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			newChanges := copyChanges(data.(*params.OfferStatusWatchResult).Changes)
			changes = w.mergeChanges(changes, newChanges)
			out = w.out
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// Changes returns a channel that will receive the changes to
// the status of an offer. The first event reflects the current
// values of these attributes.
func (w *offerStatusWatcher) Changes() watcher.OfferStatusChannel {
	return w.out
}

// machineAttachmentsWatcher will sends notifications of units entering and
// leaving the scope of a MachineStorageId, and changes to the settings of
// those units known to have entered.
type machineAttachmentsWatcher struct {
	commonWatcher
	caller                      base.APICaller
	machineAttachmentsWatcherId string
	out                         chan []watcher.MachineStorageID
}

// NewVolumeAttachmentsWatcher returns a MachineStorageIdsWatcher which
// communicates with the VolumeAttachmentsWatcher API facade to watch
// volume attachments.
func NewVolumeAttachmentsWatcher(caller base.APICaller, result params.MachineStorageIdsWatchResult) watcher.MachineStorageIDsWatcher {
	return newMachineStorageIdsWatcher("VolumeAttachmentsWatcher", caller, result)
}

// NewVolumeAttachmentPlansWatcher returns a MachineStorageIdsWatcher which
// communicates with the VolumeAttachmentPlansWatcher API facade to watch
// volume attachments.
func NewVolumeAttachmentPlansWatcher(caller base.APICaller, result params.MachineStorageIdsWatchResult) watcher.MachineStorageIDsWatcher {
	return newMachineStorageIdsWatcher("VolumeAttachmentPlansWatcher", caller, result)
}

// NewFilesystemAttachmentsWatcher returns a MachineStorageIdsWatcher which
// communicates with the FilesystemAttachmentsWatcher API facade to watch
// filesystem attachments.
func NewFilesystemAttachmentsWatcher(caller base.APICaller, result params.MachineStorageIdsWatchResult) watcher.MachineStorageIDsWatcher {
	return newMachineStorageIdsWatcher("FilesystemAttachmentsWatcher", caller, result)
}

func newMachineStorageIdsWatcher(facade string, caller base.APICaller, result params.MachineStorageIdsWatchResult) watcher.MachineStorageIDsWatcher {
	w := &machineAttachmentsWatcher{
		caller:                      caller,
		machineAttachmentsWatcherId: result.MachineStorageIdsWatcherId,
		out:                         make(chan []watcher.MachineStorageID),
	}
	w.tomb.Go(func() error {
		return w.loop(facade, result.Changes)
	})
	return w
}

func copyMachineStorageIds(src []params.MachineStorageId) []watcher.MachineStorageID {
	dst := make([]watcher.MachineStorageID, len(src))
	for i, msi := range src {
		dst[i] = watcher.MachineStorageID{
			MachineTag:    msi.MachineTag,
			AttachmentTag: msi.AttachmentTag,
		}
	}
	return dst
}

func (w *machineAttachmentsWatcher) loop(facade string, initialChanges []params.MachineStorageId) error {
	changes := copyMachineStorageIds(initialChanges)
	w.newResult = func() any { return new(params.MachineStorageIdsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, facade, w.machineAttachmentsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = copyMachineStorageIds(data.(*params.MachineStorageIdsWatchResult).Changes)
	}
}

// Changes returns a channel that will receive the IDs of machine
// storage entity attachments which have changed.
func (w *machineAttachmentsWatcher) Changes() watcher.MachineStorageIDsChannel {
	return w.out
}

// NewMigrationStatusWatcher takes the NotifyWatcherId returns by the
// MigrationSlave.Watch API and returns a watcher which will report
// status changes for any migration of the model associated with the
// API connection.
func NewMigrationStatusWatcher(caller base.APICaller, watcherId string) watcher.MigrationStatusWatcher {
	w := &migrationStatusWatcher{
		caller: caller,
		id:     watcherId,
		out:    make(chan watcher.MigrationStatus),
	}
	w.tomb.Go(w.loop)
	return w
}

type migrationStatusWatcher struct {
	commonWatcher
	caller base.APICaller
	id     string
	out    chan watcher.MigrationStatus
}

func (w *migrationStatusWatcher) loop() error {
	w.newResult = func() interface{} { return new(params.MigrationStatus) }
	w.call = makeWatcherAPICaller(w.caller, "MigrationStatusWatcher", w.id)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		var data interface{}
		var ok bool

		select {
		case data, ok = <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
		case <-w.tomb.Dying():
			return nil
		}

		inStatus := *data.(*params.MigrationStatus)
		phase, ok := migration.ParsePhase(inStatus.Phase)
		if !ok {
			return errors.Errorf("invalid phase %q", inStatus.Phase)
		}
		outStatus := watcher.MigrationStatus{
			MigrationId:    inStatus.MigrationId,
			Attempt:        inStatus.Attempt,
			Phase:          phase,
			SourceAPIAddrs: inStatus.SourceAPIAddrs,
			SourceCACert:   inStatus.SourceCACert,
			TargetAPIAddrs: inStatus.TargetAPIAddrs,
			TargetCACert:   inStatus.TargetCACert,
		}
		select {
		case w.out <- outStatus:
		case <-w.tomb.Dying():
			return nil
		}
	}
}

// Changes returns a channel that reports the latest status of the
// migration of a model.
func (w *migrationStatusWatcher) Changes() <-chan watcher.MigrationStatus {
	return w.out
}

// secretsTriggerWatcher will send notifications of changes to secret trigger times.
type secretsTriggerWatcher struct {
	commonWatcher
	caller    base.APICaller
	apiFacade string
	watcherId string
	out       chan []watcher.SecretTriggerChange
}

// NewSecretsTriggerWatcher returns a new secrets trigger watcher.
func NewSecretsTriggerWatcher(
	caller base.APICaller, result params.SecretTriggerWatchResult,
) watcher.SecretTriggerWatcher {
	w := &secretsTriggerWatcher{
		caller:    caller,
		apiFacade: "SecretsTriggerWatcher",
		watcherId: result.WatcherId,
		out:       make(chan []watcher.SecretTriggerChange),
	}
	w.newResult = func() interface{} { return new(params.SecretTriggerWatchResult) }
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop(result.Changes)
	})
	return w
}

// mergeChanges combines the changes in current and newChanges, such that we end up with
// only one change per trigger change in the result; the most recent change wins.
func (w *secretsTriggerWatcher) mergeChanges(current, newChanges []watcher.SecretTriggerChange) []watcher.SecretTriggerChange {
	chMap := make(map[string]watcher.SecretTriggerChange)
	for _, c := range current {
		chMap[c.URI.ID] = c
	}
	for _, c := range newChanges {
		chMap[c.URI.ID] = c
	}
	result := make([]watcher.SecretTriggerChange, len(chMap))
	i := 0
	for _, c := range chMap {
		result[i] = c
		i++
	}
	return result
}

func (w *secretsTriggerWatcher) loop(initialChanges []params.SecretTriggerChange) error {
	w.call = makeWatcherAPICaller(w.caller, w.apiFacade, w.watcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	copyChanges := func(changes []params.SecretTriggerChange) []watcher.SecretTriggerChange {
		result := make([]watcher.SecretTriggerChange, len(changes))
		for i, ch := range changes {
			uri, err := secrets.ParseURI(ch.URI)
			if err != nil {
				logger.Errorf(context.TODO(), "ignoring invalid secret URI: %q", ch.URI)
				continue
			}
			result[i] = watcher.SecretTriggerChange{
				URI:             uri,
				Revision:        ch.Revision,
				NextTriggerTime: ch.NextTriggerTime,
			}
		}
		return result
	}
	out := w.out
	changes := copyChanges(initialChanges)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		// Read the next change.
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			newChanges := copyChanges(data.(*params.SecretTriggerWatchResult).Changes)
			changes = w.mergeChanges(changes, newChanges)
			out = w.out
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// Changes returns a channel that will receive the changes to
// a secret trigger. The first event reflects the current
// values of these attributes.
func (w *secretsTriggerWatcher) Changes() watcher.SecretTriggerChannel {
	return w.out
}

// secretBackendRotateWatcher will send notifications of changes to secret trigger times.
type secretBackendRotateWatcher struct {
	commonWatcher
	caller    base.APICaller
	watcherId string
	out       chan []watcher.SecretBackendRotateChange
}

// NewSecretBackendRotateWatcher returns a new secret backend rotate watcher.
func NewSecretBackendRotateWatcher(
	caller base.APICaller, result params.SecretBackendRotateWatchResult,
) watcher.SecretBackendRotateWatcher {
	w := &secretBackendRotateWatcher{
		caller:    caller,
		watcherId: result.WatcherId,
		out:       make(chan []watcher.SecretBackendRotateChange),
	}
	w.newResult = func() interface{} { return new(params.SecretBackendRotateWatchResult) }
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop(result.Changes)
	})
	return w
}

// mergeChanges combines the changes in current and newChanges, such that we end up with
// only one change per trigger change in the result; the most recent change wins.
func (w *secretBackendRotateWatcher) mergeChanges(current, newChanges []watcher.SecretBackendRotateChange) []watcher.SecretBackendRotateChange {
	chMap := make(map[string]watcher.SecretBackendRotateChange)
	for _, c := range current {
		chMap[c.ID] = c
	}
	for _, c := range newChanges {
		chMap[c.ID] = c
	}
	result := make([]watcher.SecretBackendRotateChange, len(chMap))
	i := 0
	for _, c := range chMap {
		result[i] = c
		i++
	}
	return result
}

func (w *secretBackendRotateWatcher) loop(initialChanges []params.SecretBackendRotateChange) error {
	w.call = makeWatcherAPICaller(w.caller, "SecretBackendsRotateWatcher", w.watcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	copyChanges := func(changes []params.SecretBackendRotateChange) []watcher.SecretBackendRotateChange {
		result := make([]watcher.SecretBackendRotateChange, len(changes))
		for i, ch := range changes {
			result[i] = watcher.SecretBackendRotateChange{
				ID:              ch.ID,
				Name:            ch.Name,
				NextTriggerTime: ch.NextTriggerTime,
			}
		}
		return result
	}
	out := w.out
	changes := copyChanges(initialChanges)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		// Read the next change.
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			newChanges := copyChanges(data.(*params.SecretBackendRotateWatchResult).Changes)
			changes = w.mergeChanges(changes, newChanges)
			out = w.out
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// Changes returns a channel that will receive the changes to
// a secret trigger. The first event reflects the current
// values of these attributes.
func (w *secretBackendRotateWatcher) Changes() watcher.SecretBackendRotateChannel {
	return w.out
}

// SecretsRevisionWatcher will send notifications of changes to secret revisions.
type SecretsRevisionWatcher struct {
	commonWatcher
	caller    base.APICaller
	watcherId string
	out       chan []watcher.SecretRevisionChange
}

// NewSecretsRevisionWatcher returns a watcher notifying of changes to
// secret revisions.
func NewSecretsRevisionWatcher(
	caller base.APICaller, result params.SecretRevisionWatchResult,
) watcher.SecretsRevisionWatcher {
	w := &SecretsRevisionWatcher{
		caller:    caller,
		watcherId: result.WatcherId,
		out:       make(chan []watcher.SecretRevisionChange),
	}
	w.tomb.Go(func() error {
		return w.loop(result.Changes)
	})
	return w
}

// mergeChanges combines the revision in current and new, such that we end up with
// only one change per secret in the result; the most recent change wins.
func (w *SecretsRevisionWatcher) mergeChanges(current, new []watcher.SecretRevisionChange) []watcher.SecretRevisionChange {
	chMap := make(map[string]watcher.SecretRevisionChange)
	for _, c := range current {
		chMap[c.URI.ID] = c
	}
	for _, c := range new {
		chMap[c.URI.ID] = c
	}
	var result []watcher.SecretRevisionChange
	for _, c := range chMap {
		result = append(result, c)
	}
	return result
}

func (w *SecretsRevisionWatcher) loop(initialChanges []params.SecretRevisionChange) error {
	w.newResult = func() interface{} { return new(params.SecretRevisionWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "SecretsRevisionWatcher", w.watcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	copyChanges := func(changes []params.SecretRevisionChange) []watcher.SecretRevisionChange {
		var result []watcher.SecretRevisionChange
		for _, ch := range changes {
			uri, err := secrets.ParseURI(ch.URI)
			if err != nil {
				logger.Warningf(context.TODO(), "invalid secret URI: %v", ch.URI)
				continue
			}
			result = append(result, watcher.SecretRevisionChange{
				URI:      uri,
				Revision: ch.LatestRevision,
			})
		}
		return result
	}
	out := w.out
	changes := copyChanges(initialChanges)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		// Read the next change.
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			newChanges := copyChanges(data.(*params.SecretRevisionWatchResult).Changes)
			changes = w.mergeChanges(changes, newChanges)
			out = w.out
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// Changes returns a channel that will receive the changes to
// a secret revision. The first event reflects the current
// values of these attributes.
func (w *SecretsRevisionWatcher) Changes() watcher.SecretRevisionChannel {
	return w.out
}
