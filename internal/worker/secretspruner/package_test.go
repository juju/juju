// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/worker_mock.go github.com/juju/juju/internal/worker/secretspruner SecretsFacade
