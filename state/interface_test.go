// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// Ensure the interface fulfillment of interfaces without
// creating code in the binaries.
var (
	_ EntityFinder = (*State)(nil)

	_ Entity = (*Machine)(nil)
	_ Entity = (*Unit)(nil)
	_ Entity = (*UnitAgent)(nil)
	_ Entity = (*Application)(nil)
	_ Entity = (*Model)(nil)
	_ Entity = (*User)(nil)
	_ Entity = (*IPAddress)(nil)

	_ EntityWithApplication = (*Unit)(nil)

	_ Lifer = (*Machine)(nil)
	_ Lifer = (*Unit)(nil)
	_ Lifer = (*Application)(nil)
	_ Lifer = (*Relation)(nil)

	_ EnsureDeader = (*Machine)(nil)
	_ EnsureDeader = (*Unit)(nil)

	_ Remover = (*Machine)(nil)
	_ Remover = (*Unit)(nil)

	_ Authenticator = (*Machine)(nil)
	_ Authenticator = (*Unit)(nil)
	_ Authenticator = (*User)(nil)

	_ NotifyWatcherFactory = (*Machine)(nil)
	_ NotifyWatcherFactory = (*Unit)(nil)
	_ NotifyWatcherFactory = (*Application)(nil)
	_ NotifyWatcherFactory = (*Model)(nil)

	_ AgentEntity = (*Machine)(nil)
	_ AgentEntity = (*Unit)(nil)

	_ ModelAccessor = (*State)(nil)

	_ UnitsWatcher = (*Machine)(nil)
	_ UnitsWatcher = (*Application)(nil)

	_ ModelMachinesWatcher = (*State)(nil)

	_ InstanceIdGetter = (*Machine)(nil)

	_ ActionsWatcher = (*Unit)(nil)
	// TODO(jcw4): when we implement service level Actions
	// _ ActionsWatcher = (*Service)(nil)

	_ ActionReceiver = (*Unit)(nil)
	// TODO(jcw4) - use when Actions can be queued for applications.
	//_ ActionReceiver = (*Service)(nil)

	_ GlobalEntity = (*Machine)(nil)
	_ GlobalEntity = (*Unit)(nil)
	_ GlobalEntity = (*Application)(nil)
	_ GlobalEntity = (*Charm)(nil)
	_ GlobalEntity = (*Model)(nil)
)
