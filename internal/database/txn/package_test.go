// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

//go:generate go run github.com/canonical/gomock/mockgen -package txn_test -destination clock_mock_test.go github.com/juju/clock Clock,Timer
