// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package instancemutater defines workers that compares the list of lxd profiles
// applied to a machine with the list of expected profiles based on the
// application versions which should be running on the machine. In particular, it
// creates two workers from the same code with different configurations; the
// ContainerWorker, and the EnvironWorker.
//
// The ContainerWorker runs on a machine and watches for containers to be
// created on it.
//
// //	┌───────────────────────────────┐
// //	│       MACHINE                 │
// //	│                               │
// //	│                               │
// //	│   ┌──────────────────────┐    │
// //	│   │                      │    │
// //	│   │     CONTAINER        │    │
// ┌────┼───►                      │    │
// │    │   │                      │    │
// │    │   │  ┌────────────────┐  │    │
// │    │   │  │   UNIT         │  │    │
// │    │   │  │                │  │    │
// │    │   │  │                │  │    │
// │    │   │  │ ┌────────────┐ │  │    │
// │    │   │  │ │ CHARM      │ │  │    │
// │    │   │  │ │            │ │  │    │
// │    │   │  │ └─────┬──────┘ │  │    │
// │    │   │  │       │        │  │    │
// │    │   │  └───────┼────────┘  │    │
// │    │   │          │           │    │
// │    │   └──────────┼───────────┘    │
// │    │              │                │
// │    └──────────────┼────────────────┘
// │                   │
// └───────────────────┘
//
//	LXD PROFILE
//
// The EnvironWorker watches for machines in the model to be created.
//
// //	┌───────────────────────────────┐
// //	│       MACHINE                 │
// //	│                               │
// ┌────►                               │
// │    │   ┌──────────────────────┐    │
// │    │   │                      │    │
// │    │   │     CONTAINER        │    │
// │    │   │                      │    │
// │    │   │                      │    │
// │    │   │  ┌────────────────┐  │    │
// │    │   │  │   UNIT         │  │    │
// │    │   │  │                │  │    │
// │    │   │  │                │  │    │
// │    │   │  │ ┌────────────┐ │  │    │
// │    │   │  │ │ CHARM      │ │  │    │
// │    │   │  │ │            │ │  │    │
// │    │   │  │ └─────┬──────┘ │  │    │
// │    │   │  │       │        │  │    │
// │    │   │  └───────┼────────┘  │    │
// │    │   │          │           │    │
// │    │   └──────────┼───────────┘    │
// │    │              │                │
// │    └──────────────┼────────────────┘
// │                   │
// └───────────────────┘
// LXD PROFILE
//
// To understand this better with a similar mechanism, take a look at the
// provisioner worker as well.
package instancemutater
