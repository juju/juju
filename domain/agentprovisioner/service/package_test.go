// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination mock_test.go github.com/juju/juju/domain/agentprovisioner/service State,Provider
