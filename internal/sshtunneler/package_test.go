// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

//go:generate go run github.com/canonical/gomock/mockgen -package sshtunneler -destination ./service_mock_test.go github.com/juju/juju/internal/sshtunneler State,ControllerInfo,SSHDial
//go:generate go run github.com/canonical/gomock/mockgen -package sshtunneler -destination ./clock_mock_test.go github.com/juju/clock Clock
