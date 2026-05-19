// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/logwriter_mock.go github.com/juju/juju/api/logsender LogWriter
