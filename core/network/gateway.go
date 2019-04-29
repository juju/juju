// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var simulatedOS = ""

// GetDefaultRoute returns the IP address and device name of default gateway on the machine.
// If we don't support OS, or we don't have a default route, we return nothing (not an error!).
func GetDefaultRoute() (net.IP, string, error) {
	os := simulatedOS
	if os == "" {
		os = runtime.GOOS
	}
	// TODO(wpk) 2017-11-20 Add Windows support here, hence the switch.
	switch os {
	case "linux":
		return getDefaultRouteLinux()
	default:
		return nil, "", nil
	}
}

func parseIpRouteShowLine(line string) (string, map[string]string) {
	values := make(map[string]string)
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", values
	}
	to, fields := fields[0], fields[1:]
	for ; len(fields) >= 2; fields = fields[2:] {
		values[fields[0]] = fields[1]
	}
	return to, values
}

func launchIpRouteShowReal() (string, error) {
	output, err := exec.Command("ip", "route", "show").CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var launchIpRouteShow = launchIpRouteShowReal

func getDefaultRouteLinux() (net.IP, string, error) {
	output, err := launchIpRouteShow()
	if err != nil {
		return nil, "", err
	}
	logger.Tracef("ip route show output:\n%s", output)
	var defaultRouteMetric = ^uint64(0)
	var defaultRoute string
	var defaultRouteDevice string
	for _, line := range strings.Split(output, "\n") {
		to, values := parseIpRouteShowLine(line)
		logger.Tracef("parsing ip r s line to %q, values %+v ", to, values)
		if to == "default" {
			var metric = uint64(0)
			if v, ok := values["metric"]; ok {
				if i, err := strconv.ParseUint(v, 10, 64); err == nil {
					metric = i
				} else {
					return nil, "", err
				}
			}
			if metric < defaultRouteMetric {
				// We want to replace our current default route if it's valid.
				via, hasVia := values["via"]
				dev, hasDev := values["dev"]
				if hasVia || hasDev {
					defaultRouteMetric = metric
					if hasVia {
						defaultRoute = via
					} else {
						defaultRoute = ""
					}
					if hasDev {
						defaultRouteDevice = dev
					} else {
						defaultRouteDevice = ""
					}
				}
			}
		}
	}
	return net.ParseIP(defaultRoute), defaultRouteDevice, nil
}
