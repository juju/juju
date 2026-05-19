// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

//go:generate go run github.com/canonical/gomock/mockgen -package status_test -destination lease_mock_test.go github.com/juju/juju/core/lease LeaseManager
//go:generate go run github.com/canonical/gomock/mockgen -package status_test -destination leadership_mock_test.go github.com/juju/juju/domain/status/service ControllerState
