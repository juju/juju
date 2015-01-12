// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
)

var logger = loggo.GetLogger("juju.storage")

// Constraints describes a set of storage constraints.
type Constraints struct {
	// Pool is the name of the storage pool (ebs, ceph, custompool, ...)
	// that must provide the storage, or "" if the default pool should be
	// used.
	Pool string

	// Size is the minimum size of the storage in MiB.
	Size uint64

	// Count is the number of instances of the storage to create.
	Count uint64
}

var (
	poolRE  = regexp.MustCompile("^[a-zA-Z]+[-?a-zA-Z0-9]*$")
	countRE = regexp.MustCompile("^-?[0-9]+$")
	sizeRE  = regexp.MustCompile("^-?[0-9]+(?:\\.[0-9]+)?[MGTPEZY](?:i?B)?$")
)

// ParseConstraints parses the specified string and creates a
// Constraints structure.
//
// The acceptable format for storage constraints is a comma separated
// sequence of: POOL, COUNT, and SIZE, where
//
//    POOL identifies the storage pool. POOL can be a string
//    starting with a letter, followed by zero or more digits
//    or letters optionally separated by hyphens.
//
//    COUNT is a positive integer indicating how many instances
//    of the storage to create. If unspecified, and SIZE is
//    specified, COUNT defaults to 1.
//
//    SIZE describes the minimum size of the storage instances to
//    create. SIZE is a floating point number and multiplier from
//    the set (M, G, T, P, E, Z, Y), which are all treated as
//    powers of 1024.
func ParseConstraints(s string) (Constraints, error) {
	var cons Constraints
	fields := strings.Split(s, ",")
	for _, field := range fields {
		if field == "" {
			continue
		}
		if isValidPoolName(field) {
			if cons.Pool != "" {
				logger.Warningf("pool name is already set to %q, ignoring %q", cons.Pool, field)
			} else {
				cons.Pool = field
			}
			continue
		}
		if count, ok, err := parseCount(field); ok {
			if err != nil {
				return cons, errors.Annotate(err, "cannot parse count")
			}
			cons.Count = count
			continue
		}
		if size, ok, err := parseSize(field); ok {
			if err != nil {
				return cons, errors.Annotate(err, "cannot parse size")
			}
			cons.Size = size
			continue
		}
		logger.Warningf("ignoring unknown storage constraint %q", field)
	}
	if cons.Count == 0 && cons.Size > 0 {
		cons.Count = 1
	}
	return cons, nil
}

func isValidPoolName(s string) bool {
	return poolRE.MatchString(s)
}

func parseCount(s string) (uint64, bool, error) {
	if !countRE.MatchString(s) {
		return 0, false, nil
	}
	var n uint64
	var err error
	if s[0] == '-' {
		goto bad
	}
	n, err = strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false, nil
	}
	if n > 0 {
		return n, true, nil
	}
bad:
	return 0, true, errors.Errorf("count must be greater than zero, got %q", s)
}

func parseSize(s string) (uint64, bool, error) {
	if !sizeRE.MatchString(s) {
		return 0, false, nil
	}
	size, err := utils.ParseSize(s)
	if err != nil {
		return 0, true, err
	}
	return size, true, nil
}
