package config

import (
	"io/ioutil"
	"runtime"
	"strings"
)

var CurrentSeries = readSeries("/etc/lsb-release") // current Ubuntu release name.   
var CurrentArch = ubuntuArch(runtime.GOARCH)

func readSeries(releaseFile string) string {
	data, err := ioutil.ReadFile(releaseFile)
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		const p = "DISTRIB_CODENAME="
		if strings.HasPrefix(line, p) {
			return strings.Trim(line[len(p):], "\t '\"")
		}
	}
	return "unknown"
}

func ubuntuArch(arch string) string {
	if arch == "386" {
		arch = "i386"
	}
	return arch
}
