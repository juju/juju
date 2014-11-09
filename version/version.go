// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The version package implements version parsing.
// It also acts as guardian of the current client Juju version number.
package version

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/juju/arch"
)

// The presence and format of this constant is very important.
// The debian/rules build recipe uses this value for the version
// number of the release package.
const version = "1.22-alpha1"

// The version that we switched over from old style numbering to new style.
var switchOverVersion = MustParse("1.19.9")

// lsbReleaseFile is the name of the file that is read in order to determine
// the release version of ubuntu.
var lsbReleaseFile = "/etc/lsb-release"

var osVers = mustOSVersion()

// Current gives the current version of the system.  If the file
// "FORCE-VERSION" is present in the same directory as the running
// binary, it will override this.
var Current = Binary{
	Number: MustParse(version),
	Series: osVers,
	Arch:   arch.HostArch(),
	OS:     MustOSFromSeries(osVers),
}

var Compiler = runtime.Compiler

func init() {
	toolsDir := filepath.Dir(os.Args[0])
	v, err := ioutil.ReadFile(filepath.Join(toolsDir, "FORCE-VERSION"))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "WARNING: cannot read forced version: %v\n", err)
		}
		return
	}
	Current.Number = MustParse(strings.TrimSpace(string(v)))
}

// Number represents a juju version.  When bugs are fixed the patch number is
// incremented; when new features are added the minor number is incremented
// and patch is reset; and when compatibility is broken the major version is
// incremented and minor and patch are reset.  The build number is
// automatically assigned and has no well defined sequence.  If the build
// number is greater than zero or the tag is non-empty it indicates that the
// release is still in development.  For versions older than 1.19.3,
// development releases were indicated by an odd Minor number of any non-zero
// build number.
type Number struct {
	Major int
	Minor int
	Tag   string
	Patch int
	Build int
}

// Zero is occasionally convenient and readable.
// Please don't change its value.
var Zero = Number{}

// Binary specifies a binary version of juju.
type Binary struct {
	Number
	Series string
	Arch   string
	OS     OSType
}

func (v Binary) String() string {
	return fmt.Sprintf("%v-%s-%s", v.Number, v.Series, v.Arch)
}

// GetBSON turns v into a bson.Getter so it can be saved directly
// on a MongoDB database with mgo.
func (v Binary) GetBSON() (interface{}, error) {
	return v.String(), nil
}

// SetBSON turns v into a bson.Setter so it can be loaded directly
// from a MongoDB database with mgo.
func (vp *Binary) SetBSON(raw bson.Raw) error {
	var s string
	err := raw.Unmarshal(&s)
	if err != nil {
		return err
	}
	v, err := ParseBinary(s)
	if err != nil {
		return err
	}
	*vp = v
	return nil
}

func (v Binary) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

func (vp *Binary) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseBinary(s)
	if err != nil {
		return err
	}
	*vp = v
	return nil
}

// GetYAML implements goyaml.Getter
func (v Binary) GetYAML() (tag string, value interface{}) {
	return "", v.String()
}

// SetYAML implements goyaml.Setter
func (vp *Binary) SetYAML(tag string, value interface{}) bool {
	vstr := fmt.Sprintf("%v", value)
	if vstr == "" {
		return false
	}
	v, err := ParseBinary(vstr)
	if err != nil {
		return false
	}
	*vp = v
	return true
}

var (
	binaryPat = regexp.MustCompile(`^(\d{1,9})\.(\d{1,9})(\.|-(\w+))(\d{1,9})(\.\d{1,9})?-([^-]+)-([^-]+)$`)
	numberPat = regexp.MustCompile(`^(\d{1,9})\.(\d{1,9})(\.|-(\w+))(\d{1,9})(\.\d{1,9})?$`)
)

// MustParse parses a version and panics if it does
// not parse correctly.
func MustParse(s string) Number {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// MustParseBinary parses a binary version and panics if it does
// not parse correctly.
func MustParseBinary(s string) Binary {
	v, err := ParseBinary(s)
	if err != nil {
		panic(err)
	}
	return v
}

// ParseBinary parses a binary version of the form "1.2.3-series-arch".
func ParseBinary(s string) (Binary, error) {
	m := binaryPat.FindStringSubmatch(s)
	if m == nil {
		return Binary{}, fmt.Errorf("invalid binary version %q", s)
	}
	var v Binary
	v.Major = atoi(m[1])
	v.Minor = atoi(m[2])
	v.Tag = m[4]
	v.Patch = atoi(m[5])
	if m[6] != "" {
		v.Build = atoi(m[6][1:])
	}
	v.Series = m[7]
	v.Arch = m[8]
	operatingSystem, err := GetOSFromSeries(v.Series)
	if err != nil {
		return Binary{}, err
	}
	v.OS = operatingSystem
	return v, nil
}

// Parse parses the version, which is of the form 1.2.3
// giving the major, minor and release versions
// respectively.
func Parse(s string) (Number, error) {
	m := numberPat.FindStringSubmatch(s)
	if m == nil {
		return Number{}, fmt.Errorf("invalid version %q", s)
	}
	var v Number
	v.Major = atoi(m[1])
	v.Minor = atoi(m[2])
	v.Tag = m[4]
	v.Patch = atoi(m[5])
	if m[6] != "" {
		v.Build = atoi(m[6][1:])
	}
	return v, nil
}

// atoi is the same as strconv.Atoi but assumes that
// the string has been verified to be a valid integer.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (v Number) String() string {
	var s string
	if v.Tag == "" {
		s = fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	} else {
		s = fmt.Sprintf("%d.%d-%s%d", v.Major, v.Minor, v.Tag, v.Patch)
	}
	if v.Build > 0 {
		s += fmt.Sprintf(".%d", v.Build)
	}
	return s
}

// Compare returns -1, 0 or 1 depending on whether
// v is less than, equal to or greater than w.
func (v Number) Compare(w Number) int {
	if v == w {
		return 0
	}
	less := false
	switch {
	case v.Major != w.Major:
		less = v.Major < w.Major
	case v.Minor != w.Minor:
		less = v.Minor < w.Minor
	case v.Tag != w.Tag:
		switch {
		case v.Tag == "":
			less = false
		case w.Tag == "":
			less = true
		default:
			less = v.Tag < w.Tag
		}
	case v.Patch != w.Patch:
		less = v.Patch < w.Patch
	case v.Build != w.Build:
		less = v.Build < w.Build
	}
	if less {
		return -1
	}
	return 1
}

// GetBSON turns v into a bson.Getter so it can be saved directly
// on a MongoDB database with mgo.
func (v Number) GetBSON() (interface{}, error) {
	return v.String(), nil
}

// SetBSON turns v into a bson.Setter so it can be loaded directly
// from a MongoDB database with mgo.
func (vp *Number) SetBSON(raw bson.Raw) error {
	var s string
	err := raw.Unmarshal(&s)
	if err != nil {
		return err
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*vp = v
	return nil
}

func (v Number) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

func (vp *Number) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*vp = v
	return nil
}

// GetYAML implements goyaml.Getter
func (v Number) GetYAML() (tag string, value interface{}) {
	return "", v.String()
}

// SetYAML implements goyaml.Setter
func (vp *Number) SetYAML(tag string, value interface{}) bool {
	vstr := fmt.Sprintf("%v", value)
	if vstr == "" {
		return false
	}
	v, err := Parse(vstr)
	if err != nil {
		return false
	}
	*vp = v
	return true
}

func isOdd(x int) bool {
	return x%2 != 0
}

// IsDev returns whether the version represents a development version. A
// version with a tag or a nonzero build component is considered to be a
// development version.  Versions older than or equal to 1.19.3 (the switch
// over time) check for odd minor versions.
func (v Number) IsDev() bool {
	if v.Compare(switchOverVersion) <= 0 {
		return isOdd(v.Minor) || v.Build > 0
	}
	return v.Tag != "" || v.Build > 0
}

// ReleaseVersion looks for the value of DISTRIB_RELEASE in the content of
// the lsbReleaseFile.  If the value is not found, the file is not found, or
// an error occurs reading the file, an empty string is returned.
func ReleaseVersion() string {
	content, err := ioutil.ReadFile(lsbReleaseFile)
	if err != nil {
		return ""
	}
	const prefix = "DISTRIB_RELEASE="
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], "\t '\"")
		}
	}
	return ""
}

// ParseMajorMinor takes an argument of the form "major.minor" and returns ints major and minor.
func ParseMajorMinor(vers string) (int, int, error) {
	parts := strings.Split(vers, ".")
	major, err := strconv.Atoi(parts[0])
	minor := -1
	if err != nil {
		return -1, -1, fmt.Errorf("invalid major version number %s: %v", parts[0], err)
	}
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return -1, -1, fmt.Errorf("invalid minor version number %s: %v", parts[1], err)
		}
	} else if len(parts) > 2 {
		return -1, -1, fmt.Errorf("invalid major.minor version number %s", vers)
	}
	return major, minor, nil
}
