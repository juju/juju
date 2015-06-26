// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
)

// Filter is responsible for delivering events relevant to a unit agent in a
// form that can be consumed conveniently.
type Filter interface {

	// Stop shuts down the filter and returns any error encountered in the process.
	Stop() error

	// Dead returns a channel that will close when the filter has shut down.
	Dead() <-chan struct{}

	// Wait blocks until the filter has shut down, and returns any error
	// encountered in the process.
	Wait() error

	// Kill causes the filter to start shutting down if it has not already done so.
	Kill()

	// UnitDying returns a channel which is closed when the Unit enters a Dying state.
	UnitDying() <-chan struct{}

	// UpgradeEvents returns a channel that will receive a new charm URL whenever an
	// upgrade is indicated. Events should not be read until the baseline state
	// has been specified by calling WantUpgradeEvent.
	UpgradeEvents() <-chan *charm.URL

	// ResolvedEvents returns a channel that may receive a ResolvedMode when the
	// unit's Resolved value changes, or when an event is explicitly requested.
	// A ResolvedNone state will never generate events, but ResolvedRetryHooks and
	// ResolvedNoHooks will always be delivered as described.
	ResolvedEvents() <-chan params.ResolvedMode

	// MeterStatusEvents returns a channel that will receive a signal when the unit's
	// meter status changes.
	MeterStatusEvents() <-chan struct{}

	// ConfigEvents returns a channel that will receive a signal whenever the service's
	// configuration changes, or when an event is explicitly requested.
	ConfigEvents() <-chan struct{}

	// ActionEvents returns a channel that will receive a signal whenever the unit
	// receives new Actions.
	ActionEvents() <-chan string

	// RelationsEvents returns a channel that will receive the ids of all the service's
	// relations whose Life status has changed.
	RelationsEvents() <-chan []int

	// StorageEvents returns a channel that will receive the tags of all the unit's
	// associated storage instances whose Life status has changed.
	StorageEvents() <-chan []names.StorageTag

	// WantUpgradeEvent controls whether the filter will generate upgrade
	// events for unforced service charm changes.
	WantUpgradeEvent(mustForce bool)

	// SetCharm notifies the filter that the unit is running a new
	// charm. It causes the unit's charm URL to be set in state, and the
	// following changes to the filter's behaviour:
	//
	// * Upgrade events will only be generated for charms different to
	//   that supplied;
	// * A fresh relations event will be generated containing every relation
	//   the service is participating in;
	// * A fresh configuration event will be generated, and subsequent
	//   events will only be sent in response to changes in the version
	//   of the service's settings that is specific to that charm.
	//
	// SetCharm blocks until the charm URL is set in state, returning any
	// error that occurred.
	SetCharm(curl *charm.URL) error

	// WantResolvedEvent indicates that the filter should send a resolved event
	// if one is available.
	WantResolvedEvent()

	// ClearResolved notifies the filter that a resolved event has been handled
	// and should not be reported again.
	ClearResolved() error

	// DiscardConfigEvent indicates that the filter should discard any pending
	// config event.
	DiscardConfigEvent()

	// LeaderSettingsEvents returns a channel that will receive an event whenever
	// there is a leader settings change. Events can be temporarily suspended by
	// calling WantLeaderSettingsEvents(false), and then reenabled by calling
	// WantLeaderSettingsEvents(true)
	LeaderSettingsEvents() <-chan struct{}

	// DiscardLeaderSettingsEvent can be called to discard any pending
	// LeaderSettingsEvents. This is used by code that saw a LeaderSettings change,
	// and has been prepping for a response. Just before they request the current
	// LeaderSettings, they can discard any other pending changes, since they know
	// they will be handling all changes that have occurred before right now.
	DiscardLeaderSettingsEvent()

	// WantLeaderSettingsEvents can be used to enable/disable events being sent on
	// the LeaderSettingsEvents() channel. This is used when an agent notices that
	// it is the leader, it wants to disable getting events for changes that it is
	// generating. Calling this with sendEvents=false disables getting change
	// events. Calling this with sendEvents=true will enable future changes, and
	// queues up an immediate event so that the agent will refresh its information
	// for any events it might have missed while it thought it was the leader.
	WantLeaderSettingsEvents(sendEvents bool)
}
