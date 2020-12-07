/*
 * Copyright (C) 2014-2015 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package progress_test

import (
	"bytes"
	"fmt"

	"github.com/golang/mock/gomock"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/progress"
	"github.com/juju/juju/charmhub/progress/mocks"
)

type ProgressTestSuite struct{}

var _ = gc.Suite(&ProgressTestSuite{})

func (ts *ProgressTestSuite) testNotify(c *gc.C, buf *bytes.Buffer, t progress.Meter, desc, expected string) {
	t.Notify("blah blah")

	c.Check(buf.String(), gc.Equals, expected, gc.Commentf(desc))
}

func (ts *ProgressTestSuite) TestQuietNotify(c *gc.C) {
	buf := new(bytes.Buffer)
	ts.testNotify(c, buf, progress.NewQuietMeter(buf), "quiet", "blah blah\n")
}

func (ts *ProgressTestSuite) TestANSINotify(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	term := mocks.NewMockTerminal(ctrl)
	term.EXPECT().Width().Return(33)

	ec := progress.DefaultEscapeChars()
	buf := new(bytes.Buffer)
	expected := fmt.Sprint("\r", ec.ExitAttributeMode, ec.ClrEOL, "blah blah\n")
	ts.testNotify(c, buf, progress.NewANSIMeter(buf, term, ec), "ansi", expected)
}
