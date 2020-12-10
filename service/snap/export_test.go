// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

var (
	DefaultChannel           = defaultChannel
	DefaultConfinementPolicy = defaultConfinementPolicy
)

func SetServiceConfigDir(s *Service, dir string) {
	s.configDir = dir
}
