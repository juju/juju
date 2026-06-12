// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

//go:generate go run github.com/canonical/gomock/mockgen -package uniter_test -destination stepped_mock_test.go github.com/juju/juju/internal/worker/uniter Stepped

// Stepped is used by tests only.
type Stepped interface {
	Stepped(s any)
}
