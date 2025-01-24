// Copyright 2025 Canonical Ltd. Licensed under the AGPLv3, see LICENCE file for
// details.
//
// Package eventmultiplexer defines a worker that receives stream terms from the
// internal/changestream/stream worker and sends them as events to the
// appropriate event subscriptions.
//
// The eventmultiplexer sends these terms asynchronously, allowing all
// subscribers to work on the same term at the same time.
//
// Terms are processed sequentially. This creates the potential for the
// eventmultiplexer to get blocked by a subscriber if a subscriber takes too
// long to process or consume a term. To prevent that, the eventmultiplexer
// unsubscribes the slow subscriber from the next term processing, and it is up
// to the subscriber whether to resubscribe or start from scratch. For example,
// assuming a context as below with four terms, the eventmultiplexer will wait
// for all subscribers to consume TERM 1 before sending TERM 2 and, if a
// subscriber is slow to consume TERM 3, the eventmultiplexer will unsubscribe
// it from subsequent terms, then continue to TERM 4.
//
//  ┌───────────┐ ┌───────────┐ ┌───────────────────────────┐ ┌───────────┐
//  │           │ │           │ │                           │ │           │
//  │  TERM 1   │ │  TERM 2   │ │  TERM 3                   │ │  TERM 4   │
//  │           │ │           │ │                           │ │           │
//  └───────────┘ └───────────┘ └───────────────────────────┘ └───────────┘

package eventmultiplexer
