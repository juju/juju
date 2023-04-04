// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

// eq returns a gomock.Matcher that pretty formats mismatching arguments.
func eq(want any) gomock.Matcher {
	return gomock.GotFormatterAdapter(
		gomock.GotFormatterFunc(
			func(got interface{}) string {
				whole := pretty.Sprint(got)
				delta := pretty.Diff(got, want)
				return strings.Join(append([]string{whole}, delta...), "\n")
			}),
		gomock.WantFormatter(
			gomock.StringerFunc(func() string {
				return pretty.Sprint(want)
			}),
			gomock.Eq(want),
		),
	)
}
