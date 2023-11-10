// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package Instancemutater contains brokers for environ and machine containers.
// The machine container is a worker that watches all the model machines that
// can create LXD containers.  If any of those machines become provisioned then the
// instancemutater will then start watching units and applications that have a charm with an LXD
// profile, and validates and applies the profile onto the container that the unit is running on.
//
//     ┌────────────────────────────────┐
//     │                                │
//     │ MACHINE                        │
//     │                                │
//     │   ┌────────────────────────┐   │
//     │   │                        │   │
//     │   │ CONTAINER              │   │
//     │   │                        │   │
//     │   │  ┌───────────────────┐ │   │
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
//     LXD PROFILE
//
// The environ broker inside the instancemutater on the other hand watches the machine units and
// applications that have a charm with an LXD profile.
// It validates and applies the profile onto the host machine via the environ,
// that the unit is/running/on.
//
//
//     ┌────────────────────────────────┐
//     │                                │
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
//     LXD PROFILE

package instancemutater
