// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

func BackupsConfigValues(config BackupsConfig) (DBInfo, Paths) {
	conf := config.(*backupsConfig)
	return conf.dbInfo, conf.paths
}
