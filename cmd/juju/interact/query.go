// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// QueryVerify writes a question to w and waits for an answer to be read from
// scanner.  It will pass the answer into the verify function.  Verify, if
// non-nil, should check the answer for validity, returning an error that will
// be written out to the user, or nil if answer is valid.
//
// This function takes a scanner rather than an io.Reader to avoid the case
// where the scanner reads past the delimiter and thus might lose data.  It is
// expected that this method will be used repeatedly with the same scanner if
// multiple queries are required.
func QueryVerify(question []byte, scanner *bufio.Scanner, w io.Writer, verify func(string) error) (answer string, err error) {
	defer fmt.Fprint(w, "\n")
	for {
		if _, err = w.Write(question); err != nil {
			return "", err
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		answer = scanner.Text()
		if verify == nil {
			return answer, nil
		}
		err := verify(answer)
		// valid answer, return it!
		if err == nil {
			return answer, nil
		}
		// invalid answer, inform user of problem and retry.
		_, err = fmt.Fprint(w, err, "\n\n")
		if err != nil {
			return "", err
		}
	}
}

// MatchOptions returns a function that performs a case insensitive comparison
// against the given list of options.  To make a verification function that
// accepts an empty default, include an empty string in the list.
func MatchOptions(options []string, err error) func(string) error {
	return func(s string) error {
		for _, opt := range options {
			if strings.ToLower(opt) == strings.ToLower(s) {
				return nil
			}
		}
		return err
	}
}

// FindMatch does a case-insensitive search of the given options and returns the
// matching option.  Found reports whether s was found in the options.
func FindMatch(s string, options []string) (match string, found bool) {
	for _, opt := range options {
		if strings.ToLower(opt) == strings.ToLower(s) {
			return opt, true
		}
	}
	return "", false
}
