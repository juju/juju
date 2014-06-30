// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/utils/exec"
)

func osVersion() string {
	return getWinVersion()
}

func getWinVersion() string {
	var com exec.RunParams
	com.Commands = `(gwmi Win32_OperatingSystem).Name.Split('|')[0]`
	out, _ := exec.RunCommands(com)
	if out.Code != 0 {
		return "unknown"
	}
	serie := strings.TrimSpace(string(out.Stdout))
	if val, ok := windowsVersions[serie]; ok {
		return val
	}
	for key, value := range windowsVersions {
		reg := regexp.MustCompile(fmt.Sprintf("^%s", key))
		match := reg.MatchString(serie)
		if match {
			return value
		}
	}
	return "unknown"
}
