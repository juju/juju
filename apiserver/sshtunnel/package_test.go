// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

//go:generate go run github.com/canonical/gomock/mockgen -package sshtunnel -destination package_mock_test.go github.com/juju/juju/apiserver/sshtunnel TerminatingServerFactory
//go:generate go run github.com/canonical/gomock/mockgen -package sshtunnel -destination metrics_mock_test.go github.com/juju/juju/apiserver/sshtunnel MetricsCollector
