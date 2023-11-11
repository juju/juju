// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package instancemutater contains brokers for environ and machine containers.
// The machine container is a worker that watches all the model machines that
// can create LXD containers.If any of those machines become provisioned,
// the instancemutater will then start watching units and applications that
// have a charm with an LXD profile, and validate and apply the profile onto
// the container that the unit is running on.
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
