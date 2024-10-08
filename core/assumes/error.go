// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

var (
	indentRegex = regexp.MustCompile("(?m)^")
)

// RequirementsNotSatisfiedError indicates that the set of features supported
// by a model cannot satisfy an "assumes" expression tree as specified in the
// charm metadata.
type RequirementsNotSatisfiedError struct {
	message string
}

// IsRequirementsNotSatisfiedError returns true if err is a
// RequirementsNotSatisfiedError.
func IsRequirementsNotSatisfiedError(err error) bool {
	_, is := errors.Cause(err).(*RequirementsNotSatisfiedError)
	return is
}

// requirementsNotSatisfied constructs a RequirementsNotSatisfiedError error
// by combining the specified error message and a pretty-printed list of error
// messages that correspond to conflicts between the supported features and
// the required set of features from an "assumes" expression.
//
// The constructor recursively visits the error list items and collects the
// unique set of feature names and descriptions referenced by each error. A
// sorted list of feature name/description pairs will be appended at the end
// of the error to make it more user-friendly.
func requirementsNotSatisfied(message string, errList []error) *RequirementsNotSatisfiedError {
	var buf bytes.Buffer
	buf.WriteString(
		notSatisfiedError(message, errList...).Error(),
	)

	notSatFeatureDescrs := make(map[string]string)
	for _, nestedErr := range errList {
		for featName, featDescr := range notSatisfiedFeatureSet(nestedErr) {
			notSatFeatureDescrs[featName] = featDescr
		}
	}

	// Create a footer section where we list description for the features
	// names that were not matched.
	var featNames = set.NewStrings()
	for featName, featDescr := range notSatFeatureDescrs {
		if featDescr != "" {
			featNames.Add(featName)
		}
	}

	return &RequirementsNotSatisfiedError{
		message: buf.String(),
	}
}

// notSatisfiedFeatureSet recursive scans an error value (which should be either
// a notSatisfiedErr or featureErr value) and returns a set with the feature
// names that could not be satisfied and their user-friendly descriptions.
func notSatisfiedFeatureSet(err error) map[string]string {
	set := make(map[string]string)
	switch t := err.(type) {
	case *notSatisfiedErr:
		for _, nestedErr := range t.errList {
			for featName, featDescr := range notSatisfiedFeatureSet(nestedErr) {
				set[featName] = featDescr
			}
		}
	case *featureErr:
		set[t.featureName] = t.featureDescr
	}

	return set
}

// Error returns the error message associated with this error.
func (err *RequirementsNotSatisfiedError) Error() string {
	return err.message
}

// notSatisfiedErr is a type of multi-error that pretty-prints the nested
// error list when its Error() method is invoked.
type notSatisfiedErr struct {
	message string
	errList []error
}

func notSatisfiedError(message string, errors ...error) *notSatisfiedErr {
	return &notSatisfiedErr{
		message: message,
		errList: errors,
	}
}

func (err *notSatisfiedErr) Error() string {
	if len(err.errList) == 0 {
		return err.message
	}

	var buf bytes.Buffer
	buf.WriteString(err.message)
	buf.WriteRune('\n')

	for _, nestedErr := range err.errList {
		// Stringify and indent each error
		indentedErr := indentRegex.ReplaceAllString(
			"- "+strings.Trim(
				nestedErr.Error(),
				"\n",
			),
			"  ",
		)
		buf.WriteString(indentedErr)
		buf.WriteRune('\n')
	}

	return buf.String()
}

// featureErr indicates a feature-level conflict (i.e. feature not present or
// feature version mismatch) that prevents an assumes feature expression from
// being satisfied.
type featureErr struct {
	message      string
	featureName  string
	featureDescr string
}

func featureError(featureName, featureDescr, format string, args ...interface{}) *featureErr {
	return &featureErr{
		message:      fmt.Sprintf(format, args...),
		featureName:  featureName,
		featureDescr: featureDescr,
	}
}

func (err *featureErr) Error() string {
	return err.message
}
