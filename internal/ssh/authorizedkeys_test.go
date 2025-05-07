// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

type authorizedKeysSuite struct {
}

var _ = tc.Suite(&authorizedKeysSuite{})

// TestGetCommonUserPublicKeys is asserting a range of filesystem configurations
// that we are likely to come across in a users .ssh directory. This is
// asserting that after processing these directories we get back a list of
// expected public keys.
func (*authorizedKeysSuite) TestGetCommonUserPublicKeys(c *tc.C) {
	tests := []struct {
		Name        string
		Description string
		FS          fstest.MapFS
		Expected    []string
	}{
		{
			Name:        "test_unknown_files_ignored",
			Description: "test that unknown files encountered in the filesystem are ignored from the final result",
			FS: fstest.MapFS{
				"id_ed25519.pub": &fstest.MapFile{
					Data: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is"),
					Mode: fs.ModePerm,
				},
				"regular-file.txt": &fstest.MapFile{
					Data: []byte("random data"),
					Mode: fs.ModePerm,
				},
				"id_rsa.pub": &fstest.MapFile{
					Data: []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com"),
					Mode: fs.ModePerm,
				},
			},
			Expected: []string{
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
				"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com",
			},
		},
		{
			Name:        "test_happy_path",
			Description: "test the happy path of common public keys and where there is one file and no other anomalies to deal with",
			FS: fstest.MapFS{
				"id_ecdsa.pub": &fstest.MapFile{
					Data: []byte("ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com"),
					Mode: fs.ModePerm,
				},
			},
			Expected: []string{
				"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com",
			},
		},
		{
			Name:        "test_no_valid_files",
			Description: "test that if there are no valid files to read no public keys are returned",
			FS: fstest.MapFS{
				"dump.txt": &fstest.MapFile{
					Data: []byte("some data"),
					Mode: fs.ModePerm,
				},
			},
			Expected: []string{},
		},
	}

	for i, test := range tests {
		keys, err := GetCommonUserPublicKeys(context.Background(), test.FS)
		c.Assert(err, jc.ErrorIsNil, tc.Commentf("unexpected error for test %d %q", i, test.Name))
		slices.Sort(test.Expected)
		slices.Sort(keys)
		c.Assert(keys, tc.DeepEquals, test.Expected)
	}
}

// TestGetFileSystemPublicKeys is testing a set of filesystems to check that we
// correctly identify all of the ssh public keys and return the file contents as
// a slice.
func (*authorizedKeysSuite) TestGetFileSystemPublicKeys(c *tc.C) {
	tests := []struct {
		Description string
		Name        string
		FS          fstest.MapFS
		Expected    []string
	}{
		{
			Name:        "test_non_pub_files_ignored",
			Description: "test that any files not ending in .pub are ignored",
			FS: fstest.MapFS{
				"1.pub": &fstest.MapFile{
					Data: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is"),
					Mode: fs.ModePerm,
				},
				"regular-file.txt": &fstest.MapFile{
					Data: []byte("random data"),
					Mode: fs.ModePerm,
				},
				"2.pub": &fstest.MapFile{
					Data: []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com"),
					Mode: fs.ModePerm,
				},
			},
			Expected: []string{
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
				"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com",
			},
		},
		{
			Name:        "test_no_valid_files",
			Description: "test that if no valid public key files are found no results are returned",
			FS: fstest.MapFS{
				"dump.txt": &fstest.MapFile{
					Data: []byte("some data"),
					Mode: fs.ModePerm,
				},
			},
			Expected: []string{},
		},
	}

	for i, test := range tests {
		keys, err := GetFileSystemPublicKeys(context.Background(), test.FS)
		c.Assert(err, jc.ErrorIsNil, tc.Commentf("unexpected error for test %d", i))
		slices.Sort(test.Expected)
		slices.Sort(keys)
		c.Assert(keys, tc.DeepEquals, test.Expected)
	}
}

// TestSplitAuthorizedKeysFile is testing authorized keys splitting based on the
// the raw contents from a file.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysFile(c *tc.C) {
	fileStr := `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is
# This is a comment line for some reason
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is
		# This is another comment line indented with two tabs
`
	file := strings.NewReader(fileStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter('\n', file)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})

	file = strings.NewReader(fileStr)
	keys, err = SplitAuthorizedKeysReader(file)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestSplitAuthorizedKeysConfig is testing authorized keys splitting based on
// the raw contents that we are likely to encounter with a config string where
// instead of newlines we use the ';' delimiter.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysConfig(c *tc.C) {
	configStr := `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is;# This is a comment line for some reason;ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is;# This is another comment line indented with two tabs`
	configReader := strings.NewReader(configStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter(';', configReader)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestMakeAuthorizedKeysString is asserting that for a given set of keys they
// are written out in a standard compliant way to be an authorized_keys file.
func (*authorizedKeysSuite) TestMakeAuthorizedKeysString(c *tc.C) {
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}

	authorized := MakeAuthorizedKeysString(keys)
	c.Check(authorized, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}

// TestWriteAuthorizedKeys is asserting that for a given set of keys they are
// written out in a standard compliant way to the writer.
func (*authorizedKeysSuite) TestWriteAuthorizedKeys(c *tc.C) {
	builder := strings.Builder{}
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}
	WriteAuthorizedKeys(&builder, keys)
	c.Check(builder.String(), tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}
