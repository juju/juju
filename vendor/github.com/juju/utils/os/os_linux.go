// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

var defaultVersionIDs = map[string]string{
	"arch": "rolling",
}

func updateOS(f string) (OSType, error) {
	values, err := ReadOSRelease(f)
	if err != nil {
		return Unknown, err
	}
	switch values["ID"] {
	case strings.ToLower(Ubuntu.String()):
		return Ubuntu, nil
	case strings.ToLower(Arch.String()):
		return Arch, nil
	case strings.ToLower(CentOS.String()):
		return CentOS, nil
	default:
		return Unknown, nil
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
	id, ok := values["ID"]
	if !ok {
		return nil, errors.New("OS release file is missing ID")
	}
	if _, ok := values["VERSION_ID"]; !ok {
		values["VERSION_ID"], ok = defaultVersionIDs[id]
		if !ok {
			return nil, errors.New("OS release file is missing VERSION_ID")
		}
	}
	return values, nil
}
