// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wrench

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/juju/loggo"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/version"
)

var (
	enabledMu sync.Mutex
	enabled   = true

	dataDir   = paths.MustSucceed(paths.DataDir(version.Current.Series))
	wrenchDir = filepath.Join(dataDir, "wrench")
	jujuUid   = os.Getuid()
)

var logger = loggo.GetLogger("juju.wrench")

// IsActive returns true if a "wrench" of a certain category and
// feature should be "dropped in the works".
//
// This function may be called at specific points in the Juju codebase
// to introduce otherwise hard to induce failure modes for the
// purposes of manual or CI testing. The "<juju_datadir>/wrench/"
// directory will be checked for "wrench files" which this function
// looks for.
//
// Wrench files are line-based, with each line indicating some
// (mis-)feature to enable for a given part of the code. The should be
// created on the host where the fault should be triggered.
//
// For example, /var/lib/juju/wrench/machine-agent could contain:
//
//   refuse-upgrade
//   fail-api-server-start
//
// The caller need not worry about errors. Any errors that occur will
// be logged and false will be returned.
func IsActive(category, feature string) bool {
	if !IsEnabled() {
		return false
	}
	if !checkWrenchDir(wrenchDir) {
		return false
	}
	fileName := filepath.Join(wrenchDir, category)
	if !checkWrenchFile(category, feature, fileName) {
		return false
	}

	wrenchFile, err := os.Open(fileName)
	if err != nil {
		logger.Errorf("unable to read wrench data for %s/%s (ignored): %v",
			category, feature, err)
		return false
	}
	defer wrenchFile.Close()
	lines := bufio.NewScanner(wrenchFile)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == feature {
			logger.Warningf("wrench for %s/%s is active", category, feature)
			return true
		}
	}
	if err := lines.Err(); err != nil {
		logger.Errorf("error while reading wrench data for %s/%s (ignored): %v",
			category, feature, err)
	}
	return false
}

// SetEnabled turns the wrench feature on or off globally.
//
// If false is given, all future IsActive calls will unconditionally
// return false. If true is given, all future IsActive calls will
// return true for active wrenches.
//
// The previous value for the global wrench enable flag is returned.
func SetEnabled(next bool) bool {
	enabledMu.Lock()
	defer enabledMu.Unlock()
	previous := enabled
	enabled = next
	return previous
}

// IsEnabled returns true if the wrench feature is turned on globally.
func IsEnabled() bool {
	enabledMu.Lock()
	defer enabledMu.Unlock()
	return enabled
}

var stat = os.Stat // To support patching

func checkWrenchDir(dirName string) bool {
	dirinfo, err := stat(dirName)
	if err != nil {
		logger.Debugf("couldn't read wrench directory: %v", err)
		return false
	}
	if !isOwnedByJujuUser(dirinfo) {
		logger.Errorf("wrench directory has incorrect ownership - wrench "+
			"functionality disabled (%s)", wrenchDir)
		return false
	}
	return true
}

func checkWrenchFile(category, feature, fileName string) bool {
	fileinfo, err := stat(fileName)
	if err != nil {
		logger.Debugf("no wrench data for %s/%s (ignored): %v",
			category, feature, err)
		return false
	}
	if !isOwnedByJujuUser(fileinfo) {
		logger.Errorf("wrench file for %s/%s has incorrect ownership "+
			"- ignoring %s", category, feature, fileName)
		return false
	}
	// Windows is not fully POSIX compliant
	if runtime.GOOS != "windows" {
		if fileinfo.Mode()&0022 != 0 {
			logger.Errorf("wrench file for %s/%s should only be writable by "+
				"owner - ignoring %s", category, feature, fileName)
			return false
		}
	}
	return true
}
