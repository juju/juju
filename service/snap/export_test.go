// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

func SetServiceConfigDir(s *Service, dir string) {
	s.configDir = dir
}
