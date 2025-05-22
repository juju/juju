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
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/output/progress"
	"github.com/juju/juju/core/output/progress/mocks"
)

type ProgressTestSuite struct{}

func TestProgressTestSuite(t *stdtesting.T) {
	tc.Run(t, &ProgressTestSuite{})
}

func (ts *ProgressTestSuite) testNotify(c *tc.C, buf *bytes.Buffer, t progress.Meter, desc, expected string) {
	t.Notify("blah blah")

	c.Check(buf.String(), tc.Equals, expected, tc.Commentf(desc))
}

func (ts *ProgressTestSuite) TestQuietNotify(c *tc.C) {
	buf := new(bytes.Buffer)
	ts.testNotify(c, buf, progress.NewQuietMeter(buf), "quiet", "blah blah\n")
}

func (ts *ProgressTestSuite) TestANSINotify(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	term := mocks.NewMockTerminal(ctrl)
	term.EXPECT().Width().Return(33)

	ec := progress.DefaultEscapeChars()
	buf := new(bytes.Buffer)
	expected := fmt.Sprint("\r", ec.ExitAttributeMode, ec.ClrEOL, "blah blah\n")
	ts.testNotify(c, buf, progress.NewANSIMeter(buf, term, ec, clock.WallClock), "ansi", expected)
}
