// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

//go:generate go run github.com/canonical/gomock/mockgen -package sshproxy -destination tracker_mock_test.go github.com/juju/juju/apiserver/internal/handlers/sshproxy TunnelTracker
