// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package progress_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination ./mocks/term_mock.go github.com/juju/juju/core/output/progress Terminal
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination ./mocks/clock_mock.go github.com/juju/clock Clock
