// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer_test

//go:generate go run go.uber.org/mock/mockgen -typed -package containerizer -destination bridgepolicy_mock_test.go github.com/juju/juju/internal/network/containerizer Container,Address,Subnet,LinkLayerDevice
