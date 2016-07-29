// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"fmt"

	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// InfoResponse is sent by the charm store in response to charm-info requests.
type InfoResponse struct {
	CanonicalURL string   `json:"canonical-url,omitempty"`
	Revision     int      `json:"revision"` // Zero is valid. Can't omitempty.
	Sha256       string   `json:"sha256,omitempty"`
	Digest       string   `json:"digest,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// EventResponse is sent by the charm store in response to charm-event requests.
type EventResponse struct {
	Kind     string   `json:"kind"`
	Revision int      `json:"revision"` // Zero is valid. Can't omitempty.
	Digest   string   `json:"digest,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Time     string   `json:"time,omitempty"`
}

// ResourceResult holds the resources for a given charm and any error
// encountered in retrieving them.
type ResourceResult struct {
	Resources []resource.Resource
	Err       error
}

// NotFoundError represents an error indicating that the requested data wasn't found.
type NotFoundError struct {
	msg string
}

func (e *NotFoundError) Error() string {
	return e.msg
}

func repoNotFound(path string) error {
	return &NotFoundError{fmt.Sprintf("no repository found at %q", path)}
}

func entityNotFound(curl *charm.URL, repoPath string) error {
	return &NotFoundError{fmt.Sprintf("entity not found in %q: %s", repoPath, curl)}
}

// CharmNotFound returns an error indicating that the
// charm at the specified URL does not exist.
func CharmNotFound(url string) error {
	return &NotFoundError{
		msg: "charm not found: " + url,
	}
}

// BundleNotFound returns an error indicating that the
// bundle at the specified URL does not exist.
func BundleNotFound(url string) error {
	return &NotFoundError{
		msg: "bundle not found: " + url,
	}
}

// InvalidPath returns an invalidPathError.
func InvalidPath(path string) error {
	return &invalidPathError{path}
}

// invalidPathError represents an error indicating that the requested
// charm or bundle path is not valid as a charm or bundle path.
type invalidPathError struct {
	path string
}

func (e *invalidPathError) Error() string {
	return fmt.Sprintf("path %q can not be a relative path", e.path)
}

func IsInvalidPathError(err error) bool {
	_, ok := err.(*invalidPathError)
	return ok
}
