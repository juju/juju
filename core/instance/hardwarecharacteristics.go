// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/scanner"
	"unicode"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/errors"
)

// HardwareCharacteristics represents the characteristics of the instance (if known).
// Attributes that are nil are unknown or not supported.
type HardwareCharacteristics struct {
	// Arch is the architecture of the processor.
	Arch *string `json:"arch,omitempty" yaml:"arch,omitempty"`

	// Mem is the size of RAM in megabytes.
	Mem *uint64 `json:"mem,omitempty" yaml:"mem,omitempty"`

	// RootDisk is the size of the disk in megabytes.
	RootDisk *uint64 `json:"root-disk,omitempty" yaml:"rootdisk,omitempty"`

	// RootDiskSource is where the disk storage resides.
	RootDiskSource *string `json:"root-disk-source,omitempty" yaml:"rootdisksource,omitempty"`

	// CpuCores is the number of logical cores the processor has.
	CpuCores *uint64 `json:"cpu-cores,omitempty" yaml:"cpucores,omitempty"`

	// CpuPower is a relative representation of the speed of the processor.
	CpuPower *uint64 `json:"cpu-power,omitempty" yaml:"cpupower,omitempty"`

	// Tags is a list of strings that identify the machine.
	Tags *[]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// AvailabilityZone defines the zone in which the machine resides.
	AvailabilityZone *string `json:"availability-zone,omitempty" yaml:"availabilityzone,omitempty"`

	// VirtType is the virtualisation type of the instance.
	VirtType *string `json:"virt-type,omitempty" yaml:"virttype,omitempty"`
}

// quoteIfNeeded quotes s (according to Go string quoting rules) if it
// contains a comma or quote or whitespace character, otherwise it returns the
// original string.
func quoteIfNeeded(s string) string {
	i := strings.IndexFunc(s, func(c rune) bool {
		return c == ',' || c == '"' || unicode.IsSpace(c)
	})
	if i < 0 {
		// No space or comma or quote in string, return as is
		return s
	}
	return strconv.Quote(s)
}

func (hc HardwareCharacteristics) String() string {
	var strs []string
	if hc.Arch != nil {
		strs = append(strs, fmt.Sprintf("arch=%s", quoteIfNeeded(*hc.Arch)))
	}
	if hc.CpuCores != nil {
		strs = append(strs, fmt.Sprintf("cores=%d", *hc.CpuCores))
	}
	if hc.CpuPower != nil {
		strs = append(strs, fmt.Sprintf("cpu-power=%d", *hc.CpuPower))
	}
	if hc.Mem != nil {
		strs = append(strs, fmt.Sprintf("mem=%dM", *hc.Mem))
	}
	if hc.RootDisk != nil {
		strs = append(strs, fmt.Sprintf("root-disk=%dM", *hc.RootDisk))
	}
	if hc.RootDiskSource != nil {
		strs = append(strs, fmt.Sprintf("root-disk-source=%s", quoteIfNeeded(*hc.RootDiskSource)))
	}
	if hc.Tags != nil && len(*hc.Tags) > 0 {
		escapedTags := make([]string, len(*hc.Tags))
		for i, tag := range *hc.Tags {
			escapedTags[i] = quoteIfNeeded(tag)
		}
		strs = append(strs, fmt.Sprintf("tags=%s", strings.Join(escapedTags, ",")))
	}
	if hc.AvailabilityZone != nil && *hc.AvailabilityZone != "" {
		strs = append(strs, fmt.Sprintf("availability-zone=%s", quoteIfNeeded(*hc.AvailabilityZone)))
	}
	if hc.VirtType != nil && *hc.VirtType != "" {
		strs = append(strs, fmt.Sprintf("virt-type=%s", quoteIfNeeded(*hc.VirtType)))
	}
	return strings.Join(strs, " ")
}

// Clone returns a copy of the hardware characteristics.
func (hc *HardwareCharacteristics) Clone() *HardwareCharacteristics {
	if hc == nil {
		return nil
	}
	clone := *hc
	if hc.Tags != nil {
		tags := make([]string, len(*hc.Tags))
		copy(tags, *hc.Tags)
		clone.Tags = &tags
	}
	return &clone
}

// MustParseHardware constructs a HardwareCharacteristics from the supplied arguments,
// as Parse, but panics on failure.
func MustParseHardware(args ...string) HardwareCharacteristics {
	hc, err := ParseHardware(args...)
	if err != nil {
		panic(err)
	}
	return hc
}

// ParseHardware constructs a HardwareCharacteristics from the supplied arguments,
// each of which must contain only spaces and name=value pairs. If any
// name is specified more than once, an error is returned.
func ParseHardware(args ...string) (HardwareCharacteristics, error) {
	hc := HardwareCharacteristics{}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		for arg != "" {
			var err error
			arg, err = hc.parseField(arg)
			if err != nil {
				return hc, errors.Capture(err)
			}
			arg = strings.TrimSpace(arg)
		}
	}
	return hc, nil
}

// parseField parses a single name=value (or name="value") field into the
// corresponding field of the receiver.
func (hc *HardwareCharacteristics) parseField(s string) (rest string, err error) {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return s, errors.Errorf("malformed characteristic %q", s)
	}
	name, rest := s[:eq], s[eq+1:]

	switch name {
	case "tags":
		// Tags is a multi-valued field (comma separated)
		var values []string
		values, rest, err = parseMulti(rest)
		if err != nil {
			return rest, errors.Errorf("%s: %v", name, err)
		}
		err = hc.setTags(values)
	default:
		// All other fields are single-valued
		var value string
		value, rest, err = parseSingle(rest, " ")
		if err != nil {
			return rest, errors.Errorf("%s: %v", name, err)
		}
		switch name {
		case "arch":
			err = hc.setArch(value)
		case "cores":
			err = hc.setCpuCores(value)
		case "cpu-power":
			err = hc.setCpuPower(value)
		case "mem":
			err = hc.setMem(value)
		case "root-disk":
			err = hc.setRootDisk(value)
		case "root-disk-source":
			err = hc.setRootDiskSource(value)
		case "availability-zone":
			err = hc.setAvailabilityZone(value)
		case "virt-type":
			err = hc.setVirtType(value)
		default:
			return rest, errors.Errorf("unknown characteristic %q", name)
		}
	}
	if err != nil {
		return rest, errors.Errorf("bad %q characteristic: %v", name, err)
	}
	return rest, nil
}

// parseSingle parses a single (optionally quoted) value from s and returns
// the value and the remainder of the string.
func parseSingle(s string, seps string) (value, rest string, err error) {
	if len(s) > 0 && s[0] == '"' {
		value, rest, err = parseQuotedString(s)
		if err != nil {
			return "", rest, errors.Capture(err)
		}
	} else {
		sepPos := strings.IndexAny(s, seps)
		value = s
		if sepPos >= 0 {
			value, rest = value[:sepPos], value[sepPos:]
		}
	}
	return value, rest, nil
}

// parseMulti parses multiple (optionally quoted) comma-separated values from s
// and returns the values and the remainder of the string.
func parseMulti(s string) (values []string, rest string, err error) {
	needComma := false
	rest = s
	for rest != "" && rest[0] != ' ' {
		if needComma {
			if rest[0] != ',' {
				return values, rest, errors.New("expected comma after quoted value")
			}
			rest = rest[1:]
		}
		needComma = true

		var value string
		value, rest, err = parseSingle(rest, ", ")
		if err != nil {
			return values, rest, errors.Capture(err)
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values, rest, nil
}

// parseQuotedString parses a string name=value argument, returning the
// unquoted value and the remainder of the string.
func parseQuotedString(input string) (value, rest string, err error) {
	// Use text/scanner to find end of quoted string
	var s scanner.Scanner
	s.Init(strings.NewReader(input))
	s.Mode = scanner.ScanStrings
	s.Whitespace = 0
	var errMsg string
	s.Error = func(s *scanner.Scanner, msg string) {
		// Record first error
		if errMsg == "" {
			errMsg = msg
		}
	}
	tok := s.Scan()
	rest = input[s.Pos().Offset:]
	if s.ErrorCount > 0 {
		return "", rest, errors.Errorf("parsing quoted string: %s", errMsg)
	}
	if tok != scanner.String {
		// Shouldn't happen; we only asked for strings
		return "", rest, errors.Errorf("parsing quoted string: unexpected token %s", scanner.TokenString(tok))
	}

	// And then strconv to unquote it (oddly, text/scanner doesn't unquote)
	unquoted, err := strconv.Unquote(s.TokenText())
	if err != nil {
		// Shouldn't happen; scanner should only return valid quoted strings
		return "", rest, errors.Errorf("parsing quoted string: %v", err)
	}
	return unquoted, rest, nil
}

func (hc *HardwareCharacteristics) setArch(str string) error {
	if hc.Arch != nil {
		return errors.Errorf("already set")
	}
	if str != "" && !arch.IsSupportedArch(str) {
		return errors.Errorf("%q not recognized", str)
	}
	hc.Arch = &str
	return nil
}

func (hc *HardwareCharacteristics) setCpuCores(str string) (err error) {
	if hc.CpuCores != nil {
		return errors.Errorf("already set")
	}
	hc.CpuCores, err = parseUint64(str)
	return
}

func (hc *HardwareCharacteristics) setCpuPower(str string) (err error) {
	if hc.CpuPower != nil {
		return errors.Errorf("already set")
	}
	hc.CpuPower, err = parseUint64(str)
	return
}

func (hc *HardwareCharacteristics) setMem(str string) (err error) {
	if hc.Mem != nil {
		return errors.Errorf("already set")
	}
	hc.Mem, err = parseSize(str)
	return
}

func (hc *HardwareCharacteristics) setRootDisk(str string) (err error) {
	if hc.RootDisk != nil {
		return errors.Errorf("already set")
	}
	hc.RootDisk, err = parseSize(str)
	return
}

func (hc *HardwareCharacteristics) setRootDiskSource(str string) (err error) {
	if hc.RootDiskSource != nil {
		return errors.Errorf("already set")
	}
	if str != "" {
		hc.RootDiskSource = &str
	}
	return
}

func (hc *HardwareCharacteristics) setTags(strs []string) (err error) {
	if hc.Tags != nil {
		return errors.Errorf("already set")
	}
	if len(strs) > 0 {
		hc.Tags = &strs
	}
	return
}

func (hc *HardwareCharacteristics) setAvailabilityZone(str string) error {
	if hc.AvailabilityZone != nil {
		return errors.Errorf("already set")
	}
	if str != "" {
		hc.AvailabilityZone = &str
	}
	return nil
}

func (hc *HardwareCharacteristics) setVirtType(str string) error {
	if hc.VirtType != nil {
		return errors.Errorf("already set")
	}
	// TODO (stickupkid): We potentially will want to allow "" to be a valid
	// container virt-type, converting all empty strings to the default instance
	// type. For now, allow LXD to fallback to the default instance type.
	if str == "" {
		return nil
	}
	if _, err := ParseVirtType(str); err != nil {
		return errors.Capture(err)
	}
	hc.VirtType = &str
	return nil
}

func parseUint64(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		val, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return nil, errors.Errorf("must be a non-negative integer")
		}
		value = val
	}
	return &value, nil
}

func parseSize(str string) (*uint64, error) {
	var value uint64
	if str != "" {
		mult := 1.0
		if m, ok := mbSuffixes[str[len(str)-1:]]; ok {
			str = str[:len(str)-1]
			mult = m
		}
		val, err := strconv.ParseFloat(str, 64)
		if err != nil || val < 0 {
			return nil, errors.Errorf("must be a non-negative float with optional M/G/T/P suffix")
		}
		val *= mult
		value = uint64(math.Ceil(val))
	}
	return &value, nil
}

var mbSuffixes = map[string]float64{
	"M": 1,
	"G": 1024,
	"T": 1024 * 1024,
	"P": 1024 * 1024 * 1024,
}
