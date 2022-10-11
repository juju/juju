// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type utilSuite struct {
}

var _ = gc.Suite(&utilSuite{})

func (u *utilSuite) TestDataOrFile(c *gc.C) {
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
			c.Assert(err, jc.ErrorIsNil)
			n, err := f.Write(test.fileContents)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(n, gc.Equals, len(test.fileContents))
		}

		r, err := dataOrFile(test.dataContents, fileName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(r, jc.DeepEquals, test.expectedContents)
	}
}

func (u *utilSuite) TestStringOrFile(c *gc.C) {
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
			c.Assert(err, jc.ErrorIsNil)
			n, err := f.Write([]byte(test.fileContents))
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(n, gc.Equals, len(test.fileContents))
		}

		r, err := stringOrFile(test.dataContents, fileName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(r, jc.DeepEquals, test.expectedContents)
	}
}
