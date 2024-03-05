// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package introspection defines the worker that can report internal agent state
// through the use of a machine local socket.
//
// The most interesting endpoints at this stage are:
//
//   - /debug/pprof/goroutine?debug=1:
//     prints out all the goroutines in the agent
//   - /debug/pprof/heap?debug=1:
//     prints out the heap profile
package introspection
