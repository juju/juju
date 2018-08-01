// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Note: this code was copied from github.com/juju/utils.

// Package httputil holds utility functions related to net/http.
package httputil

import (
	"errors"
	"strings"
)

// RelativeURLPath returns a relative URL path that is lexically
// equivalent to targpath when interpreted by url.URL.ResolveReference.
// On success, the returned path will always be non-empty and relative
// to basePath, even if basePath and targPath share no elements.
//
// It is assumed that both basePath and targPath are normalized
// (have no . or .. elements).
//
// An error is returned if basePath or targPath are not absolute paths.
func RelativeURLPath(basePath, targPath string) (string, error) {
	if !strings.HasPrefix(basePath, "/") {
		return "", errors.New("non-absolute base URL")
	}
	if !strings.HasPrefix(targPath, "/") {
		return "", errors.New("non-absolute target URL")
	}
	baseParts := strings.Split(basePath, "/")
	targParts := strings.Split(targPath, "/")

	// For the purposes of dotdot, the last element of
	// the paths are irrelevant. We save the last part
	// of the target path for later.
	lastElem := targParts[len(targParts)-1]
	baseParts = baseParts[0 : len(baseParts)-1]
	targParts = targParts[0 : len(targParts)-1]

	// Find the common prefix between the two paths:
	var i int
	for ; i < len(baseParts); i++ {
		if i >= len(targParts) || baseParts[i] != targParts[i] {
			break
		}
	}
	dotdotCount := len(baseParts) - i
	targOnly := targParts[i:]
	result := make([]string, 0, dotdotCount+len(targOnly)+1)
	for i := 0; i < dotdotCount; i++ {
		result = append(result, "..")
	}
	result = append(result, targOnly...)
	result = append(result, lastElem)
	final := strings.Join(result, "/")
	if final == "" {
		// If the final result is empty, the last element must
		// have been empty, so the target was slash terminated
		// and there were no previous elements, so "."
		// is appropriate.
		final = "."
	}
	return final, nil
}
