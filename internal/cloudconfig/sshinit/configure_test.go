// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/cloudconfig/sshinit"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
)

type configureSuite struct {
	coretesting.BaseSuite
}

func TestConfigureSuite(t *testing.T) {
	tc.Run(t, &configureSuite{})
}

// fakeExitError mimics the go-crypto client's remote exit error, which
// reports the remote exit status through ExitStatus.
type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string {
	return fmt.Sprintf("Process exited with status %d", e.code)
}

func (e *fakeExitError) ExitStatus() int {
	return e.code
}

func (s *configureSuite) TestSetupTrapFired(c *tc.C) {
	for i, test := range []struct {
		about string
		err   error
		want  bool
	}{
		{"success", nil, true},
		{"remote exit", utils.NewRcPassthroughError(7), true},
		{"wrapped remote exit", errors.Errorf("wrapped: %w", utils.NewRcPassthroughError(7)), true},
		{"go-crypto remote exit", &fakeExitError{code: 7}, true},
		{"ssh(1) client failure", utils.NewRcPassthroughError(255), false},
		{"go-crypto client failure", &fakeExitError{code: 255}, false},
		{"sudo cannot start wrapper", utils.NewRcPassthroughError(126), false},
		{"sudo command not found", utils.NewRcPassthroughError(127), false},
		{"connection refused", errors.New("ssh: connect to host refused"), false},
		{"cancelled before start", context.Canceled, false},
	} {
		c.Logf("test %d: %s", i, test.about)
		c.Check(sshinit.SetupTrapFired(test.err), tc.Equals, test.want)
	}
}
