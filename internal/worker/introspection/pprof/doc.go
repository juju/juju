// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package pprof is a fork of net/http/pprof modified to communicate
// over a unix socket.
//
// # Changes from the original version
//
//   - This fork does not automatically register itself with the default
//     net/http ServeMux.
//   - To start the pprof handler, see the Start method in socket.go.
//
// ---------------------------------------------------------------
//
// Package pprof serves via its HTTP server runtime profiling data
// in the format expected by the pprof visualization tool.
// For more information about pprof, see
// http://code.google.com/p/google-perftools/.
//
// The package is typically only imported for the side effect of
// registering its HTTP handlers.
// The handled paths all begin with /debug/pprof/.
//
// To use pprof, link this package into your program:
//
//	import _ "net/http/pprof"
//
// If your application is not already running an http server, you
// need to start one.  Add "net/http" and "log" to your imports and
// the following code to your main function:
//
//	go func() {
//		log.Println(http.ListenAndServe("localhost:6060", nil))
//	}()
//
// Then use the pprof tool to look at the heap profile:
//
//	go tool pprof http://localhost:6060/debug/pprof/heap
//
// Or to look at a 30-second CPU profile:
//
//	go tool pprof http://localhost:6060/debug/pprof/profile
//
// Or to look at the goroutine blocking profile:
//
//	go tool pprof http://localhost:6060/debug/pprof/block
//
// Or to collect a 5-second execution trace:
//
//	wget http://localhost:6060/debug/pprof/trace?seconds=5
//
// To view all available profiles, open http://localhost:6060/debug/pprof/
// in your browser.
//
// For a study of the facility in action, visit
//
//	https://blog.golang.org/2011/06/profiling-go-programs.html
package pprof
