// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"
)

type utilSuite struct {
}

func TestUtilSuite(t *stdtesting.T) { tc.Run(t, &utilSuite{}) }
func (u *utilSuite) TestDataOrFile(c *tc.C) {
	tests := []struct {
		dataContents     []byte
		fileContents     []byte
		expectedContents []byte
	}{
		{
			dataContents:     []byte("test"),
			expectedContents: []byte("test"),
		},
		{
			dataContents:     []byte{},
			fileContents:     []byte("test"),
			expectedContents: []byte("test"),
		},
		{
			dataContents:     []byte{},
			fileContents:     []byte{},
			expectedContents: []byte{},
		},
	}

	for _, test := range tests {
		fileName := ""
		if len(test.fileContents) > 0 {
			f, err := os.CreateTemp("", "")
			fileName = f.Name()
			c.Assert(err, tc.ErrorIsNil)
			n, err := f.Write(test.fileContents)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(n, tc.Equals, len(test.fileContents))
		}

		r, err := dataOrFile(test.dataContents, fileName)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(r, tc.DeepEquals, test.expectedContents)
	}
}

func (u *utilSuite) TestStringOrFile(c *tc.C) {
	tests := []struct {
		dataContents     string
		fileContents     string
		expectedContents string
	}{
		{
			dataContents:     "test",
			expectedContents: "test",
		},
		{
			fileContents:     "test",
			expectedContents: "test",
		},
		{
			expectedContents: "",
		},
	}

	for _, test := range tests {
		fileName := ""
		if test.fileContents != "" {
			f, err := os.CreateTemp("", "")
			fileName = f.Name()
			c.Assert(err, tc.ErrorIsNil)
			n, err := f.Write([]byte(test.fileContents))
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(n, tc.Equals, len(test.fileContents))
		}

		r, err := stringOrFile(test.dataContents, fileName)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(r, tc.DeepEquals, test.expectedContents)
	}
}
