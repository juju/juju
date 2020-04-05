// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !skip_state_tests

package state

// runStateTests controls whether to run the state tests - in this
// case the skip_state_tests build tag hasn't been set so they'll be
// run as normal.
const runStateTests = true
