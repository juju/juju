// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"
)

func osVersion() (string, error) {
	var com exec.RunParams
	com.Commands = `(gwmi Win32_OperatingSystem).Name.Split('|')[0]`
	out, err := exec.RunCommands(com)
	if err != nil || out.Code != 0 {
		return "unknown", err
	}
	series := strings.TrimSpace(string(out.Stdout))
	if val, ok := windowsVersions[series]; ok {
		return val, err
	}
	for key, value := range windowsVersions {
		if strings.HasPrefix(series, key) {
			return value, nil
		}
	}
	return "unknown", errors.Errorf("unknown series %q", series)
}
