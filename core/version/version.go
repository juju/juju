// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	semversion "github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// The presence and format of this constant is very important.
// The debian/rules build recipe uses this value for the version
// number of the release package.
const version = "4.0-beta6"

// UserAgentVersion defines a user agent version used for communication for
// outside resources.
const UserAgentVersion = "Juju/" + version

const (
	// TreeStateDirty when the build was made with a dirty checkout.
	TreeStateDirty = "dirty"
	// TreeStateClean when the build was made with a clean checkout.
	TreeStateClean = "clean"
	// TreeStateArchive when the build was made outside of a git checkout.
	TreeStateArchive = "archive"
)

// The version that we switched over from old style numbering to new style.
var switchOverVersion = semversion.MustParse("1.19.9")

// build is a string representing this build of Juju's number.
//
// NOTE: This is injected by the build system. In Makefile, we override
// this value with the value of the JUJU_BUILD_NUMBER environment variable.
var build string

// OfficialBuild is a monotonic number injected by Jenkins.
var OfficialBuild = mustParseBuildInt(build)

// Current gives the current version of the system.
//
// If the file "FORCE-VERSION" is present in the same directory as the running
// binary, it will override this.
//
// We also later set the build number with build, if it is set.
var Current = semversion.MustParse(version)

// Compiler is the go compiler used to build the binary.
var Compiler = runtime.Compiler

// GitCommit represents the git commit sha used to build the binary.
//
// NOTE: This is injected by the build system. In Makefile, we override
// this value with the value of the GIT_COMMIT environment variable.
var GitCommit string

// GitTreeState is "clean" when built from a working copy that matches the
// GitCommit treeish.
//
// NOTE: This is injected by the build system. In Makefile, we override
// this value with the value of the GIT_TREE_STATE environment variable.
var GitTreeState string = TreeStateDirty

// GoBuildTags is the build tags used to build the binary.
//
// NOTE: This is injected by the build system. In Makefile, we override
// this value with the value of the FINAL_BUILD_TAGS environment variable.
var GoBuildTags string

func init() {
	defer func() {
		if Current.Build == 0 {
			// We set the Build to OfficialBuild if no build number provided in the FORCE-VERSION file.
			Current.Build = OfficialBuild
		}
	}()

	toolsDir := filepath.Dir(os.Args[0])
	v, err := os.ReadFile(filepath.Join(toolsDir, "FORCE-VERSION"))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "WARNING: cannot read forced version: %v\n", err)
		}
		return
	}
	Current = semversion.MustParse(strings.TrimSpace(string(v)))
}

func isOdd(x int) bool {
	return x%2 != 0
}

// IsDev returns whether the version represents a development version. A
// version with a tag or a nonzero build component is considered to be a
// development version.  Versions older than or equal to 1.19.3 (the switch
// over time) check for odd minor versions.
func IsDev(v semversion.Number) bool {
	if v.Compare(switchOverVersion) <= 0 {
		return isOdd(v.Minor) || v.Build > 0
	}
	return v.Tag != "" || v.Build > 0
}

func mustParseBuildInt(buildInt string) int {
	if buildInt == "" {
		return 0
	}
	i, err := strconv.Atoi(buildInt)
	if err != nil {
		panic(err)
	}
	return i
}

// CheckJujuMinVersion returns an error if the specified version to check is
// less than the current Juju version.
func CheckJujuMinVersion(toCheck semversion.Number, jujuVersion semversion.Number) (err error) {
	// It only makes sense to allow charms to specify they depend
	// on a released version of Juju. If this is a beta or rc version
	// of Juju, treat it like it's the released version to allow
	// charms to be tested prior to release.
	jujuVersion.Tag = ""
	jujuVersion.Build = 0
	if toCheck != semversion.Zero && toCheck.Compare(jujuVersion) > 0 {
		return minVersionError(toCheck, jujuVersion)
	}
	return nil
}

func minVersionError(minver, jujuver semversion.Number) error {
	err := errors.Errorf("charm's min version (%s) is higher than this juju model's version (%s)",
		minver, jujuver)

	return minJujuVersionErr{err}
}

type minJujuVersionErr struct {
	error
}

// IsMinVersionError returns true if the given error was caused by the charm
// having a minjujuversion higher than the juju model's version.
func IsMinVersionError(err error) bool {
	_, ok := errors.AsType[minJujuVersionErr](err)
	return ok
}
