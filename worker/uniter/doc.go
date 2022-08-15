// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniter is the "uniter" worker which implements the capabilities of
// the unit agent, for example running a charm's hooks in response to model
// events. The uniter worker sets up the various components which make that
// happen and then runs the top level event loop.
package uniter
