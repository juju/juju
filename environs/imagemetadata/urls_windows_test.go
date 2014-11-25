// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package imagemetadata_test

var imageTestsPlatformSpecific = []struct {
	in          string
	expected    string
	expectedErr error
}{{
	in:          "C:/home/foo",
	expected:    "file://\\\\localhost\\C$/home/foo/images",
	expectedErr: nil,
}, {
	in:          "C:/home/foo/images",
	expected:    "file://\\\\localhost\\C$/home/foo/images",
	expectedErr: nil,
}}
