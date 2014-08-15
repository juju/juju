// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

func NewBackupsConfigRaw(
	addr, user, pw, dbBinDir string, paths Paths,
) (BackupsConfig, error) {
	dbInfo, err := NewDBInfoFull(addr, user, pw, dbBinDir, "")
	if err != nil {
		return nil, err
	}
	config, err := NewBackupsConfig(dbInfo, paths)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func BackupsConfigValues(config BackupsConfig) (DBInfo, Paths) {
	conf := config.(*backupsConfig)
	return conf.dbInfo, conf.paths
}

func ExposePaths(p *paths) (root, data, startup, loggingConf, logs, ssh string) {
	return p.rootDir, p.dataDir, p.startupDir, p.loggingConfDir, p.logsDir, p.sshDir
}

func ResolvePath(p *paths, kind, relPath string) (string, error) {
	return p.resolve(kind, relPath)
}

func ReRoot(p *paths, rootDir string) *paths {
	return p.reRoot(rootDir)
}
