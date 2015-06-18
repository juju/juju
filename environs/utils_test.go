// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

type utilsSuite struct {
	testing.IsolationSuite
}

func (s *utilsSuite) TestLocalUsername(c *gc.C) {
	type test struct {
		userEnv   string
		sudoEnv   string
		userOS    string
		userOSErr error
		namespace string
		err       string
	}
	tests := []test{{
		userEnv:   "someone",
		sudoEnv:   "notroot",
		userOS:    "other",
		namespace: "someone-test",
	}, {
		userOS:    "other",
		namespace: "other-test",
	}, {
		userEnv:   "root",
		namespace: "root-test",
	}, {
		userEnv:   "root",
		sudoEnv:   "other",
		namespace: "other-test",
	}, {
		userOSErr: errors.New("oh noes"),
		err:       "failed to determine username for namespace: oh noes",
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		s.PatchEnvironment("USER", test.userEnv)
		s.PatchEnvironment("SUDO_USER", test.sudoEnv)
		s.PatchValue(environs.UserCurrent, func() (string, error) {
			return test.userOS, test.userOSErr
		})

		namespace, err := environs.LocalUsername()

		if test.err == "" {
			if c.Check(err, jc.ErrorIsNil) {
				c.Check(namespace, gc.Equals, test.namespace)
			}
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *utilsSuite) TestLocalNamespaceOkay(c *gc.C) {
	s.PatchValue(&environs.GetUsername, func() (string, error) {
		return "a", nil
	})

	namespace, err := environs.LocalNamespace("test")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(namespace, gc.Equals, "a-test")
}

func (s *utilsSuite) TestLocalNamespaceError(c *gc.C) {
	expected := errors.New("<failure>")
	s.PatchValue(&environs.GetUsername, func() (string, error) {
		return "", expected
	})

	_, err := environs.LocalNamespace("test")

	c.Check(errors.Cause(err), gc.Equals, expected)
}
