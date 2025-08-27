// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

//go:generate go run go.uber.org/mock/mockgen -package gce -destination gcemock_test.go github.com/juju/juju/internal/provider/gce ComputeService
