// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	stdos "os"
	"strings"
	"sync"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/errors"
)

var (
	// osReleaseFile is the name of the file that is read in order to determine
	// the linux type release version.
	osReleaseFile = "/etc/os-release"
	osOnce        sync.Once
	os            ostype.OSType // filled in by the first call to hostOS
)

func hostOS() ostype.OSType {
	osOnce.Do(func() {
		var err error
		os, err = updateOS(osReleaseFile)
		if err != nil {
			panic("unable to read " + osReleaseFile + ": " + err.Error())
		}
	})
	return os
}

func updateOS(f string) (ostype.OSType, error) {
	values, err := ReadOSRelease(f)
	if err != nil {
		return ostype.Unknown, err
	}
	switch values["ID"] {
	case strings.ToLower(ostype.Ubuntu.String()):
		return ostype.Ubuntu, nil
	case strings.ToLower(ostype.CentOS.String()):
		return ostype.CentOS, nil
	default:
		return ostype.GenericLinux, nil
	}
}

// ReadOSRelease parses the information in the os-release file.
//
// See http://www.freedesktop.org/software/systemd/man/os-release.html.
func ReadOSRelease(f string) (map[string]string, error) {
	contents, err := stdos.ReadFile(f)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	releaseDetails := strings.Split(string(contents), "\n")
	for _, val := range releaseDetails {
		c := strings.SplitN(val, "=", 2)
		if len(c) != 2 {
			continue
		}
		values[c[0]] = strings.Trim(c[1], "\t '\"")
	}
	if _, ok := values["ID"]; !ok {
		return nil, errors.New("OS release file is missing ID")
	}
	if _, ok := values["VERSION_ID"]; !ok {
		return nil, errors.New("OS release file is missing VERSION_ID")
	}
	return values, nil
}
