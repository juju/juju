// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/subnet"
	coretesting "github.com/juju/juju/internal/testing"
)

type SubnetCommandSuite struct {
	BaseSubnetSuite
}

func TestSubnetCommandSuite(t *stdtesting.T) { tc.Run(t, &SubnetCommandSuite{}) }

type SubnetCommandBaseSuite struct {
	coretesting.BaseSuite

	baseCmd *subnet.SubnetCommandBase
}

func (s *SubnetCommandBaseSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.baseCmd = &subnet.SubnetCommandBase{}
}
func TestSubnetCommandBaseSuite(t *stdtesting.T) { tc.Run(t, &SubnetCommandBaseSuite{}) }
func (s *SubnetCommandBaseSuite) TestCheckNumArgs(c *tc.C) {
	threeErrors := []error{
		errors.New("first"),
		errors.New("second"),
		errors.New("third"),
	}
	twoErrors := threeErrors[:2]
	oneError := threeErrors[:1]
	threeArgs := []string{"foo", "bar", "baz"}
	twoArgs := threeArgs[:2]
	oneArg := threeArgs[:1]

	for i, errs := range [][]error{nil, oneError, twoErrors, threeErrors} {
		for j, args := range [][]string{nil, oneArg, twoArgs, threeArgs} {
			expectErr := ""
			if i > j {
				// Returned error is always the one with index equal
				// to len(args), if it exists.
				expectErr = errs[j].Error()
			}

			c.Logf("test #%d: args: %v, errors: %v -> %q", i*4+j, args, errs, expectErr)
			err := s.baseCmd.CheckNumArgs(args, errs)
			if expectErr != "" {
				c.Check(err, tc.ErrorMatches, expectErr)
			} else {
				c.Check(err, tc.ErrorIsNil)
			}
		}
	}
}

func (s *SubnetCommandBaseSuite) TestValidateCIDR(c *tc.C) {
	// We only validate the subset of accepted CIDR formats which we
	// need to support.
	for i, test := range []struct {
		about     string
		input     string
		strict    bool
		output    string
		expectErr string
	}{{
		about:  "valid IPv4 CIDR, strict=false",
		input:  "10.0.5.0/24",
		strict: false,
		output: "10.0.5.0/24",
	}, {
		about:  "valid IPv4 CIDR, struct=true",
		input:  "10.0.5.0/24",
		strict: true,
		output: "10.0.5.0/24",
	}, {
		about:  "valid IPv6 CIDR, strict=false",
		input:  "2001:db8::/32",
		strict: false,
		output: "2001:db8::/32",
	}, {
		about:  "valid IPv6 CIDR, strict=true",
		input:  "2001:db8::/32",
		strict: true,
		output: "2001:db8::/32",
	}, {
		about:  "incorrectly specified IPv4 CIDR, strict=false",
		input:  "192.168.10.20/16",
		strict: false,
		output: "192.168.0.0/16",
	}, {
		about:     "incorrectly specified IPv4 CIDR, strict=true",
		input:     "192.168.10.20/16",
		strict:    true,
		expectErr: `"192.168.10.20/16" is not correctly specified, expected "192.168.0.0/16"`,
	}, {
		about:  "incorrectly specified IPv6 CIDR, strict=false",
		input:  "2001:db8::2/48",
		strict: false,
		output: "2001:db8::/48",
	}, {
		about:     "incorrectly specified IPv6 CIDR, strict=true",
		input:     "2001:db8::2/48",
		strict:    true,
		expectErr: `"2001:db8::2/48" is not correctly specified, expected "2001:db8::/48"`,
	}, {
		about:     "empty CIDR, strict=false",
		input:     "",
		strict:    false,
		expectErr: `"" is not a valid CIDR`,
	}, {
		about:     "empty CIDR, strict=true",
		input:     "",
		strict:    true,
		expectErr: `"" is not a valid CIDR`,
	}} {
		c.Logf("test #%d: %s -> %s", i, test.about, test.expectErr)
		validated, err := s.baseCmd.ValidateCIDR(test.input, test.strict)
		if test.expectErr != "" {
			c.Check(err, tc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
		c.Check(validated, tc.Equals, test.output)
	}
}

func (s *SubnetCommandBaseSuite) TestValidateSpace(c *tc.C) {
	// We only validate a few more common invalid cases as
	// names.IsValidSpace() is separately and more extensively tested.
	for i, test := range []struct {
		about     string
		input     string
		expectErr string
	}{{
		about: "valid space - only lowercase letters",
		input: "space",
	}, {
		about: "valid space - only numbers",
		input: "42",
	}, {
		about: "valid space - only lowercase letters and numbers",
		input: "over9000",
	}, {
		about: "valid space - with dashes",
		input: "my-new-99space",
	}, {
		about:     "invalid space - with symbols",
		input:     "%in$valid",
		expectErr: `"%in\$valid" is not a valid space name`,
	}, {
		about:     "invalid space - with underscores",
		input:     "42_foo",
		expectErr: `"42_foo" is not a valid space name`,
	}, {
		about:     "invalid space - with uppercase letters",
		input:     "Not-Good",
		expectErr: `"Not-Good" is not a valid space name`,
	}, {
		about:     "empty space name",
		input:     "",
		expectErr: `"" is not a valid space name`,
	}} {
		c.Logf("test #%d: %s -> %s", i, test.about, test.expectErr)
		validated, err := s.baseCmd.ValidateSpace(test.input)
		if test.expectErr != "" {
			c.Check(err, tc.ErrorMatches, test.expectErr)
			c.Check(validated.Id(), tc.Equals, "")
		} else {
			c.Check(err, tc.ErrorIsNil)
			// When the input is valid it should stay the same.
			c.Check(validated.Id(), tc.Equals, test.input)
		}
	}
}
