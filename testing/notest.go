// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build notest

package testing

import (
	_ "unsafe"
)

// Dear Reader,
// You have found your way here because you imported github.com/juju/juju/testing
// into code that found its way into a non-test binary.
// This is bad. Please don't use test code inside a juju binary.

//go:linkname do_not_import_test_code_into_juju
func do_not_import_test_code_into_juju()

func init() {
	do_not_import_test_code_into_juju()
}
