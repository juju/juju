// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

// AutoConfigureContainerNetworking tries to set up best container networking available
// for the specific model if user hasn't set anything.
func (m *Model) AutoConfigureContainerNetworking(environ environs.BootstrapEnviron) error {
	updateAttrs := make(map[string]interface{})
	modelConfig, err := m.ModelConfig()
	if err != nil {
		return err
	}
	var fanConfigured bool
	if cnm := modelConfig.ContainerNetworkingMethod(); cnm != "local" && cnm != "provider" {
		var err error
		fanConfigured, err = m.discoverFan(environ, modelConfig, updateAttrs)
		if err != nil {
			return errors.Annotatef(err, "auto discovering fan config")
		}
	}

	if modelConfig.ContainerNetworkingMethod() != "" {
		// Do nothing, user has decided what to do
	} else if environs.SupportsContainerAddresses(context.CallContext(m.st), environ) {
		updateAttrs["container-networking-method"] = "provider"
	} else if fanConfigured {
		updateAttrs["container-networking-method"] = "fan"
	} else {
		updateAttrs["container-networking-method"] = "local"
	}
	err = m.UpdateModelConfig(updateAttrs, nil)
	return err
}

func (m *Model) discoverFan(environ environs.BootstrapEnviron, modelConfig *config.Config, updateAttrs map[string]interface{}) (bool, error) {
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		// Not a networking environ, nothing to do here
		return false, nil
	}
	fanConfig, err := modelConfig.FanConfig()
	if err != nil {
		return false, err
	}
	if len(fanConfig) != 0 {
		logger.Debugf("Not trying to autoconfigure FAN - configured already")
		return false, nil
	}
	subnets, err := netEnviron.SuperSubnets(context.CallContext(m.st))
	if errors.IsNotSupported(err) || (err == nil && len(subnets) == 0) {
		logger.Debugf("Not trying to autoconfigure FAN - SuperSubnets not supported or empty")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var outputTable []string

	fanOverlays := []string{"252.0.0.0/8", "253.0.0.0/8", "254.0.0.0/8", "250.0.0.0/8", "251.0.0.0/8"}
	fanOverlayForUnderlay := func(underlay string) string {
		_, ipNet, err := net.ParseCIDR(underlay)
		if err != nil {
			return ""
		}
		// We don't create FAN networks for IPv6 networks
		if ipNet.IP.To4() == nil {
			return ""
		}
		if ones, _ := ipNet.Mask.Size(); ones <= 8 {
			return ""
		}
		if len(fanOverlays) == 0 {
			return ""
		}
		var overlay string
		overlay, fanOverlays = fanOverlays[0], fanOverlays[1:]
		return overlay
	}
	for _, subnet := range subnets {
		overlay := fanOverlayForUnderlay(subnet)
		if overlay != "" {
			outputTable = append(outputTable, fmt.Sprintf("%s=%s", subnet, overlay))
		}
	}
	if len(outputTable) > 0 {
		updateAttrs["fan-config"] = strings.Join(outputTable, " ")
		return true, nil
	}
	return false, nil
}
