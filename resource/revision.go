// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"regexp"
	"time"

	"github.com/juju/errors"
)

// TODO(ericsnow) This file probably belongs in the charm repo.

// This revision machinery is necessary because there is more than one
// kind of revision: integers for charm store resources and dates for
// uploaded resources. Additionally, specs for uploaded resources do not
// have any revision.

// TODO(ericsnow) Type -> Kind.

// These are the recognized revision types (except for unknown).
const (
	RevisionTypeUnknown RevisionType = ""
	RevisionTypeNone    RevisionType = "no revision"
	RevisionTypeNumber  RevisionType = "number"
	RevisionTypeDate    RevisionType = "date"
)

var (
	// TODO(ericsnow) Enforce a max revision?
	numRegexp = regexp.MustCompile(`^\d+$`)
	// TODO(ericsnow) Leave the date open-ended (for extra revision info)?
	dateRegexp = regexp.MustCompile(`^\d\d\d\d-\d\d-\d\d$`)
)

var revisionTypes = map[RevisionType]func(string) bool{
	RevisionTypeNone:   func(s string) bool { return len(s) == 0 },
	RevisionTypeNumber: numRegexp.MatchString,
	// TODO(ericsnow) Use time.Parse() instead?
	RevisionTypeDate: dateRegexp.MatchString,
}

// RevisionType identifies a type of resource revision (e.g. date, int).
type RevisionType string

// Validate ensures that the revision type is correct.
func (rt RevisionType) Validate() error {
	if _, ok := revisionTypes[rt]; !ok {
		return errors.NewNotValid(nil, "unknown revision type")
	}
	return nil
}

// NoRevision indicates that the spec does not have a revision specified.
var NoRevision Revision = Revision{
	Type:  RevisionTypeNone,
	Value: "",
}

// Revision identifies a resouce revision.
type Revision struct {
	// TYpe is the kind of revision.
	Type RevisionType

	// Value is the revision value.
	Value string
}

// ParseRevision converts the provided value into a Revision. If it
// cannot be converted then false is returned.
func ParseRevision(value string) (Revision, error) {
	rev := Revision{
		Value: value,
	}

	for rt, match := range revisionTypes {
		if match(value) {
			rev.Type = rt
			return rev, nil
		}
	}

	if err := rev.Validate(); err != nil {
		return rev, errors.Trace(err)
	}
	return rev, nil
}

// String returns the printable representation of the revision.
func (rev Revision) String() string {
	return rev.Value
}

// Validate ensures that the revision is correct.
func (rev Revision) Validate() error {
	if err := rev.Type.Validate(); err != nil {
		return errors.Annotate(err, "bad revision type")
	}

	match := revisionTypes[rev.Type]
	if !match(rev.Value) {
		msg := fmt.Sprintf("invalid value %q for revision type %s", rev.Value, rev.Type)
		return errors.NewNotValid(nil, msg)
	}

	// Do more type-specific checking.
	switch rev.Type {
	case RevisionTypeDate:
		if err := checkDate(rev.Value); err != nil {
			err = errors.Annotatef(err, "invalid value %q for revision type %s", rev.Value, rev.Type)
			return errors.NewNotValid(err, "")
		}
	}

	return nil
}

func checkDate(value string) error {
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		return errors.Trace(err)
	}

	// time.Parse() only checks that the day-of-month is less than 32.
	// Consequently, February, April, June, September, and November
	// aren't checked exactly right.
	// TODO(ericsnow) Check leap years and days between EOM and 31?

	return nil
}
