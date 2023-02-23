// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build skip_state_tests

package state_test

// runStateTests controls whether to run the state tests - in this case
// the skip_state_tests build tag has been set so they'll be skipped.
const runStateTests = false
