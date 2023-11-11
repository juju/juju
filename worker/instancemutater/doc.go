// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.


// Package instancemutater defines workers that checks the list of lxd profiles
// applied to a machine against the list of expected profiles based on the
// application version which should be running on the machine. In particular, it
// creates two workers from the same code with different configurations; the
// ContainerWorker, and the EnvironWorker.
//
//	┌────────────────────────────────┐
//	│                                │
//	│ MACHINE                        │
//	│                                │
//	│   ┌────────────────────────┐   │
//	│   │                        │   │
//	│   │ CONTAINER              │   │
//	│   │                        │   │
//	│   │  ┌───────────────────┐ │   │
//
// ┌───┼───►  │                   │ │   │
// │   │   │  │ UNIT              │ │   │
// │   │   │  │ ┌───────────────┐ │ │   │
// │   │   │  │ │               │ │ │   │
// │   │   │  │ │ CHARM         │ │ │   │
// │   │   │  │ │               │ │ │   │
// │   │   │  │ └──────┬────────┘ │ │   │
// │   │   │  │        │          │ │   │
// │   │   │  └────────┼──────────┘ │   │
// │   │   │           │            │   │
// │   │   └───────────┼────────────┘   │
// │   │               │                │
// │   └───────────────┼────────────────┘
// │                   │
// │                   │
// └───────────────────┘
//
//	LXD PROFILE
//
// On the other hand, the environ broker inside the instancemutater watches
// the machine units and applications that have a charm with an LXD profile.
// It validates and applies the profile via the environ onto the host machine
// that the unit is running on.
//
//	┌────────────────────────────────┐
//	│                                │
//
// ┌───► MACHINE                        │
// │   │                                │
// │   │   ┌────────────────────────┐   │
// │   │   │                        │   │
// │   │   │ CONTAINER              │   │
// │   │   │                        │   │
// │   │   │  ┌───────────────────┐ │   │
// │   │   │  │                   │ │   │
// │   │   │  │ UNIT              │ │   │
// │   │   │  │ ┌───────────────┐ │ │   │
// │   │   │  │ │               │ │ │   │
// │   │   │  │ │ CHARM         │ │ │   │
// │   │   │  │ │               │ │ │   │
// │   │   │  │ └──────┬────────┘ │ │   │
// │   │   │  │        │          │ │   │
// │   │   │  └────────┼──────────┘ │   │
// │   │   │           │            │   │
// │   │   └───────────┼────────────┘   │
// │   │               │                │
// │   └───────────────┼────────────────┘
// │                   │
// │                   │
// └───────────────────┘
//
//	LXD PROFILE
//
// To understand this better with a similar mechanism, take a look at the
// provisioner worker as well.
package instancemutater
