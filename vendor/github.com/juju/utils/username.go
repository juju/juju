// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"os/user"

	"github.com/juju/errors"
)

// ResolveSudo returns the original username if sudo was used. The
// original username is extracted from the OS environment.
func ResolveSudo(username string) string {
	return resolveSudo(username, os.Getenv)
}

func resolveSudo(username string, getenvFunc func(string) string) string {
	if username != "root" {
		return username
	}
	// sudo was probably called, get the original user.
	if username := getenvFunc("SUDO_USER"); username != "" {
		return username
	}
	return username
}

// EnvUsername returns the username from the OS environment.
func EnvUsername() (string, error) {
	return os.Getenv("USER"), nil
}

// OSUsername returns the username of the current OS user (based on UID).
func OSUsername() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", errors.Trace(err)
	}
	return u.Username, nil
}

// ResolveUsername returns the username determined by the provided
// functions. The functions are tried in the same order in which they
// were passed in. An error returned from any of them is immediately
// returned. If an empty string is returned then that signals that the
// function did not find the username and the next function is tried.
// Once a username is found, the provided resolveSudo func (if any) is
// called with that username and the result is returned. If no username
// is found then errors.NotFound is returned.
func ResolveUsername(resolveSudo func(string) string, usernameFuncs ...func() (string, error)) (string, error) {
	for _, usernameFunc := range usernameFuncs {
		username, err := usernameFunc()
		if err != nil {
			return "", errors.Trace(err)
		}
		if username != "" {
			if resolveSudo != nil {
				if original := resolveSudo(username); original != "" {
					username = original
				}
			}
			return username, nil
		}
	}
	return "", errors.NotFoundf("username")
}

// LocalUsername determines the current username on the local host.
func LocalUsername() (string, error) {
	username, err := ResolveUsername(ResolveSudo, EnvUsername, OSUsername)
	if err != nil {
		return "", errors.Annotatef(err, "cannot get current user from the environment: %v", os.Environ())
	}
	return username, nil
}
