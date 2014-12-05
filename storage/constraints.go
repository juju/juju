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
	// Size is the preferred size of the storage in MiB.
	Size uint64

	// MinimumSize is the minimum size of the storage in MiB.
	MinimumSize uint64

	// Count is the preferred number of instances of the
	// storage to create.
	Count uint64

	// MinimumCount is the minimum number of instances of the
	// storage to create.
	MinimumCount uint64

	// Persistent indicates that the storage should be persistent
	// beyond the lifetime of the machine that it is initially
	// attached to.
	Persistent bool

	// RequirePersistent indicates that the storage must be persistent.
	RequirePersistent bool

	// IOPS is the preferred number of IOPS (I/O Operations Per Second)
	// that the storage should be capable of.
	IOPS uint64

	// MinimumIOPS is the minimum number of IOPS that the storage must
	// be capable of.
	MinimumIOPS uint64

	// Source is the name of the storage source (ebs, ceph, ...) that
	// must provide the storage, or "" if any source may be used.
	Source string
}

const (
	countSnippet      = "(?:(-?[0-9]+)x)"
	sizeSuffixSnippet = "(?:[MGTPEZY](?:i?B)?)"
	sizeSnippet       = "(-?[0-9]+(?:\\.[0-9]+)?" + sizeSuffixSnippet + "?)"
)

var countSizeRE = regexp.MustCompile("^" + countSnippet + "?" + sizeSnippet + "$")

// ParseConstraints parses the specified string and creates a
// Constraints structure.
//
// The acceptable format for storage constraints is:
//    [SOURCE:][[COUNTx]SIZE][,persistent][,iops:IOPS]
// where
//    SOURCE identifies the storage source. SOURCE can be a
//    string starting with a letter of the alphabet, followed
//    by zero or more alpha-numeric characters optionally
//    separated by hyphens.
//
//    COUNT is a positive integer indicating how many instances
//    of the storage to create. If unspecified, and SIZE is
//    specified, COUNT defaults to 1.
//
//    SIZE describes the minimum size of the storage instances to
//    create. SIZE is a floating point number and optional multiplier
//    from the set (M, G, T, P, E, Z, Y), which are all treated as
//    powers of 1024.
//
//    IOPS is a positive integer describing the minimum number of
//    IOPS the storage should be capable of. If unspecified, then
//    there is no constraint.
func ParseConstraints(s string) (Constraints, error) {
	var cons Constraints
	if i := strings.IndexRune(s, ':'); i >= 0 {
		cons.Source, s = s[:i], s[i+1:]
	}

	var countSizeMatch []string
	if i := strings.IndexRune(s, ','); i >= 0 {
		countSizeMatch = countSizeRE.FindStringSubmatch(s[:i])
		if countSizeMatch != nil {
			s = s[i+1:]
		}
	} else {
		countSizeMatch = countSizeRE.FindStringSubmatch(s)
		if countSizeMatch != nil {
			s = ""
		}
	}
	var err error
	if countSizeMatch != nil {
		if countSizeMatch[1] != "" {
			if countSizeMatch[1][0] != '-' {
				cons.Count, err = strconv.ParseUint(countSizeMatch[1], 10, 64)
				if err != nil {
					return cons, errors.Annotatef(err, "cannot parse count %q", countSizeMatch[1])
				}
			}
			if cons.Count == 0 {
				return cons, errors.Errorf("count must be greater than zero, got %q", countSizeMatch[1])
			}
		} else {
			cons.Count = 1
		}
		cons.Size, err = utils.ParseSize(countSizeMatch[2])
		if err != nil {
			return cons, errors.Annotate(err, "cannot parse size")
		}
	}

	// Remaining constraints may be in any order.
	for _, field := range strings.Split(s, ",") {
		field = strings.TrimSpace(field)
		switch {
		case field == "":
		case field == "persistent":
			cons.Persistent = true
		case strings.HasPrefix(strings.ToLower(field), "iops:"):
			value := field[len("iops:"):]
			cons.IOPS, err = strconv.ParseUint(value, 10, 64)
			if err != nil {
				return cons, errors.Annotatef(err, "cannot parse IOPS %q", value)
			}
		default:
			logger.Warningf("ignoring unknown storage constraint %q", field)
		}
	}

	cons.MinimumCount = cons.Count
	cons.MinimumSize = cons.Size
	cons.RequirePersistent = cons.Persistent
	cons.MinimumIOPS = cons.IOPS
	return cons, nil
}
