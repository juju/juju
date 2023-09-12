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
// be written out to errOut, or nil if answer is valid.
//
// This function takes a scanner rather than an io.Reader to avoid the case
// where the scanner reads past the delimiter and thus might lose data.  It is
// expected that this method will be used repeatedly with the same scanner if
// multiple queries are required.
func QueryVerify(question string, scanner *bufio.Scanner, out, errOut io.Writer, verify VerifyFunc) (answer string, err error) {
	defer fmt.Fprint(out, "\n")
	for {
		if _, err = out.Write([]byte(question)); err != nil {
			return "", err
		}

		done := !scanner.Scan()

		if done {
			if err := scanner.Err(); err != nil {
				return "", err
			}
		}
		answer = scanner.Text()
		if done && answer == "" {
			// EOF
			return "", io.EOF
		}
		if verify == nil {
			return answer, nil
		}
		ok, msg, err := verify(answer)
		if err != nil {
			return "", err
		}
		// valid answer, return it!
		if ok {
			return answer, nil
		}

		// invalid answer, inform user of problem and retry.
		if msg != "" {
			_, err := fmt.Fprintln(errOut, msg)
			if err != nil {
				return "", err
			}
		}
		_, err = errOut.Write([]byte{'\n'})
		if err != nil {
			return "", err
		}

		if done {
			// can't query any more, nothing we can do.
			return "", io.EOF
		}
	}
}

// MatchOptions returns a function that performs a case insensitive comparison
// against the given list of options.  To make a verification function that
// accepts an empty default, include an empty string in the list.
func MatchOptions(options []string, errmsg string) VerifyFunc {
	return func(s string) (ok bool, msg string, err error) {
		for _, opt := range options {
			if strings.EqualFold(opt, s) {
				return true, "", nil
			}
		}
		return false, errmsg, nil
	}
}

// FindMatch does a case-insensitive search of the given options and returns the
// matching option.  Found reports whether s was found in the options.
func FindMatch(s string, options []string) (match string, found bool) {
	for _, opt := range options {
		if strings.EqualFold(opt, s) {
			return opt, true
		}
	}
	return "", false
}
