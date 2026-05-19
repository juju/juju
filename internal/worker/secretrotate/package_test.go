// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/client_mock.go -source secretrotate.go
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher SecretTriggerWatcher
