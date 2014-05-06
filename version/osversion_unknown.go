// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !darwin,!linux

func osVersion() string {
	return "unknown"
}
