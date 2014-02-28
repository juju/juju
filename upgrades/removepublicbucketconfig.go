// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"launchpad.net/juju-core/environs/config"
)

func removePublicBucketConfig(context Context) error {
	st := context.State()
	cfg, err := st.EnvironConfig()
	if err != nil {
		return fmt.Errorf("failed to read current config: %v", err)
	}
	newAttrs := cfg.AllAttrs()
	delete(newAttrs, "public-bucket")
	delete(newAttrs, "public-bucket-region")
	newCfg, err := config.New(config.NoDefaults, newAttrs)
	if err != nil {
		return fmt.Errorf("failed to create new config: %v", err)
	}
	return st.SetEnvironConfig(newCfg, cfg)
}
