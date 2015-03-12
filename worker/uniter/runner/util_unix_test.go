// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package runner_test

var (
	// Platform specific hook name used in runner_test.go
	hookName = "something-happened"

	// Platform specific script used in runner_test.go
	echoPidScript = "echo $$ > pid"
)
