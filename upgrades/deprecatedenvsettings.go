// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func processDeprecatedEnvSettings(context Context) error {
	st := context.State()
	removeAttrs := []string{
		"public-bucket",
		"public-bucket-region",
		"public-bucket-url",
		"default-image-id",
		"default-instance-type",
		"shared-storage-port",
	}
	// TODO (wallyworld) - delete tools-url in 1.20
	// TODO (wallyworld) - delete lxc-use-clone in 1.22
	return st.UpdateEnvironConfig(map[string]interface{}{}, removeAttrs, nil)
}
