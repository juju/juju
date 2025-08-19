// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
// Copied over from https://github.com/juju/naturalsort with minor adjustments.

package naturalsort_test

import (
	"math/rand"
	"testing"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/internal/naturalsort"
)

func TestPackage(t *testing.T) {
	tc.Run(t, &naturalSortSuite{})
}

type naturalSortSuite struct {
	jujutesting.IsolationSuite
}

func (s *naturalSortSuite) TestEmpty(c *tc.C) {
	checkCorrectSort(c, []string{})
}

func (s *naturalSortSuite) TestAlpha(c *tc.C) {
	checkCorrectSort(c, []string{"abc", "bac", "cba"})
}

func (s *naturalSortSuite) TestNumVsString(c *tc.C) {
	checkCorrectSort(c, []string{"1", "a"})
}

func (s *naturalSortSuite) TestStringVsStringNum(c *tc.C) {
	checkCorrectSort(c, []string{"a", "a1"})
}

func (s *naturalSortSuite) TestCommonPrefix(c *tc.C) {
	checkCorrectSort(c, []string{"a1", "a1a", "a1b", "a2b", "a2c"})
}

func (s *naturalSortSuite) TestDifferentNumberLengths(c *tc.C) {
	checkCorrectSort(c, []string{"a1a", "a2", "a22a", "a333", "a333a", "a333b"})
}

func (s *naturalSortSuite) TestZeroPadding(c *tc.C) {
	checkCorrectSort(c, []string{"a1", "a002", "a3"})
}

func (s *naturalSortSuite) TestMixed(c *tc.C) {
	checkCorrectSort(c, []string{"1a", "a1", "a1/1", "a10", "a100"})
}

func (s *naturalSortSuite) TestSeveralNumericParts(c *tc.C) {
	checkCorrectSort(c, []string{
		"x",
		"x1",
		"x1-g0",
		"x1-g1",
		"x1-g2",
		"x1-g10",
		"x2",
		"x2-g0",
		"x2-g2",
		"x11-g0",
		"x11-g0-0",
		"x11-g0-1",
		"x11-g0-10",
		"x11-g0-11",
		"x11-g0-20",
		"x11-g0-100",
		"x11-g10-1",
		"x11-g10-10",
		"xx1",
		"xx10",
	})
}

func (s *naturalSortSuite) TestUnitNameLike(c *tc.C) {
	checkCorrectSort(c, []string{"a1/1", "a1/2", "a1/7", "a1/11", "a1/100"})
}

func (s *naturalSortSuite) TestMachineIdLike(c *tc.C) {
	checkCorrectSort(c, []string{
		"1",
		"1/lxc/0",
		"1/lxc/1",
		"1/lxc/2",
		"1/lxc/10",
		"1/lxd/0",
		"1/lxd/1",
		"1/lxd/10",
		"2",
		"11",
		"11/lxc/6",
		"11/lxc/60",
		"20",
		"21",
	})
}

func (s *naturalSortSuite) TestIPs(c *tc.C) {
	checkCorrectSort(c, []string{
		"001.001.010.121",
		"1.1.10.122",
		"001.001.010.123",
		"001.002.010.123",
		"100.001.010.123",
		"100.1.10.124",
		"100.2.10.124",
	})
}

func (s *naturalSortSuite) TestFallbackToLexicographicalSortForLargeNumbers(c *tc.C) {
	checkCorrectSort(c, []string{
		"app-23456789012345678901234567890x",
		"app-900",
		"app-903",
		"app-900000000000000000",
	})
}

func checkCorrectSort(c *tc.C, expected []string) {
	checkSort(c, expected, reverse)
	for i := 0; i < 5; i++ {
		checkSort(c, expected, shuffle)
	}
}

func checkSort(c *tc.C, expected []string, xform func([]string)) {
	input := copyStrSlice(expected)
	xform(input)
	origInput := copyStrSlice(input)
	naturalsort.Sort(input)
	c.Check(input, tc.DeepEquals, expected, tc.Commentf("input was: %#v", origInput))
}

func copyStrSlice(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func shuffle(a []string) {
	// See https://en.wikipedia.org/wiki/Fisher%E2%80%93Yates_shuffle#Modern_method
	for i := len(a) - 1; i >= 1; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

func reverse(a []string) {
	size := len(a)
	for i := 0; i < size/2; i++ {
		j := size - i - 1
		a[i], a[j] = a[j], a[i]
	}
}
