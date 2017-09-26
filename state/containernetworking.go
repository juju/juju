// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// AutoConfigureContainerNetworking tries to set up best container networking available
// for the specific model if user hasn't set anything.
func (m *Model) AutoConfigureContainerNetworking(environ environs.Environ) error {
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		return errors.NotSupportedf("fan configuration in a non-networking environ")
	}
	modelConfig, err := m.ModelConfig()
	if err != nil {
		return err
	}
	updateAttrs := make(map[string]interface{})
	fanConfigured, err := m.discoverFan(netEnviron, modelConfig, updateAttrs)
	if err != nil {
		return err
	}

	if modelConfig.ContainerNetworkingMethod() != "" {
		// Do nothing, user has decided what to do
	} else if ok, _ := netEnviron.SupportsContainerAddresses(); ok {
		updateAttrs["container-networking-method"] = "provider"
	} else if fanConfigured {
		updateAttrs["container-networking-method"] = "fan"
	} else {
		updateAttrs["container-networking-method"] = "local"
	}
	err = m.st.UpdateModelConfig(updateAttrs, nil)
	return err
}

func (m *Model) discoverFan(netEnviron environs.NetworkingEnviron, modelConfig *config.Config, updateAttrs map[string]interface{}) (bool, error) {
	fanConfig, err := modelConfig.FanConfig()
	if err != nil {
		return false, err
	}
	if len(fanConfig) != 0 {
		logger.Debugf("Not trying to autoconfigure FAN - configured already")
		return false, nil
	}
	subnets, err := netEnviron.SuperSubnets()
	if errors.IsNotSupported(err) || (err == nil && len(subnets) == 0) {
		logger.Debugf("Not trying to autoconfigure FAN - SuperSubnets not supported or empty")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var outputTable []string
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

func fanOverlayForUnderlay(overlay string) string {
	return "253.0.0.0/8"
}
