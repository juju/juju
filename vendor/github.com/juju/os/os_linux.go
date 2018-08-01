// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"errors"
	"io/ioutil"
	"strings"
	"sync"
)

var (
	// osReleaseFile is the name of the file that is read in order to determine
	// the linux type release version.
	osReleaseFile = "/etc/os-release"
	osOnce        sync.Once
	os            OSType // filled in by the first call to hostOS
)

func hostOS() OSType {
	osOnce.Do(func() {
		var err error
		os, err = updateOS(osReleaseFile)
		if err != nil {
			panic("unable to read " + osReleaseFile + ": " + err.Error())
		}
	})
	return os
}

func updateOS(f string) (OSType, error) {
	values, err := ReadOSRelease(f)
	if err != nil {
		return Unknown, err
	}
	switch values["ID"] {
	case strings.ToLower(Ubuntu.String()):
		return Ubuntu, nil
	case strings.ToLower(CentOS.String()):
		return CentOS, nil
	case strings.ToLower(OpenSUSE.String()):
		return OpenSUSE, nil
	default:
		return GenericLinux, nil
	}
}

// ReadOSRelease parses the information in the os-release file.
//
// See http://www.freedesktop.org/software/systemd/man/os-release.html.
func ReadOSRelease(f string) (map[string]string, error) {
	contents, err := ioutil.ReadFile(f)
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
	return values, nil
}
