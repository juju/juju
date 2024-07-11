// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package socketlistener provides a worker that will listen on a specified unix
// socket identified by a file descriptor. Handlers are provided to the worker
// that specify endpoints and define the action to be taken when they are
// reached.
package socketlistener
