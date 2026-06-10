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
package pprof
