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
	_ Entity = (*Service)(nil)
	_ Entity = (*Environment)(nil)
	_ Entity = (*User)(nil)
	_ Entity = (*Action)(nil)
	_ Entity = (*IPAddress)(nil)

	_ EntityWithService = (*Unit)(nil)

	_ Lifer = (*Machine)(nil)
	_ Lifer = (*Unit)(nil)
	_ Lifer = (*Service)(nil)
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
	_ NotifyWatcherFactory = (*Service)(nil)
	_ NotifyWatcherFactory = (*Environment)(nil)

	_ AgentEntity = (*Machine)(nil)
	_ AgentEntity = (*Unit)(nil)

	_ EnvironAccessor = (*State)(nil)

	_ UnitsWatcher = (*Machine)(nil)
	_ UnitsWatcher = (*Service)(nil)

	_ EnvironMachinesWatcher = (*State)(nil)

	_ InstanceIdGetter = (*Machine)(nil)

	_ ActionsWatcher = (*Unit)(nil)
	// TODO(jcw4): when we implement service level Actions
	// _ ActionsWatcher = (*Service)(nil)

	_ ActionReceiver = (*Unit)(nil)
	// TODO(jcw4) - use when Actions can be queued for Services.
	//_ ActionReceiver = (*Service)(nil)

	_ GlobalEntity = (*Machine)(nil)
	_ GlobalEntity = (*Unit)(nil)
	_ GlobalEntity = (*Service)(nil)
	_ GlobalEntity = (*Charm)(nil)
	_ GlobalEntity = (*Environment)(nil)
)
