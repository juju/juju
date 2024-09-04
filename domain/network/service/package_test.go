// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/juju/juju/core/network"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/network/service State,Provider

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// generateFanSubnetID generates a correct ID for a subnet of type fan overlay.
func generateFanSubnetID(subnetNetwork, providerID string) string {
	subnetWithDashes := strings.Replace(strings.Replace(subnetNetwork, ".", "-", -1), "/", "-", -1)
	return fmt.Sprintf("%s-%s-%s", providerID, network.InFan, subnetWithDashes)
}
