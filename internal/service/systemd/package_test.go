// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

//go:generate go run github.com/canonical/gomock/mockgen -package systemd_test -destination package_mock_test.go github.com/juju/juju/internal/service/systemd DBusAPI,FileSystemOps

// TODO (manadart 2020-01-28): Phase this out
// and generate all mocks with the command above.
//go:generate go run github.com/canonical/gomock/mockgen -package systemd -destination shims_mock_test.go github.com/juju/juju/internal/service/systemd ShimExec
