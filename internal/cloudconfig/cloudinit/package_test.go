// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

//go:generate go run go.uber.org/mock/mockgen -typed -package cloudinit_test -destination filetransporter_mock_test.go github.com/juju/juju/internal/cloudconfig/cloudinit FileTransporter
