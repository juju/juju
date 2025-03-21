// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type returnSuite struct{}

var _ = gc.Suite(&returnSuite{})

var tests = []struct {
	Name     string
	Expected string
	Input    string
}{
	// This test exists for the most basic case where we have a contrived error
	// and it isn't checked for nil before returning it with a call to
	// errors.Annotate.
	{
		Name: "basic error not checked",
		Input: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() error {
	err := errors.New("test")
	return errors.Annotate(err, "test")
}
`[1:],
		Expected: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() error {
	err := errors.New("test")
	if err != nil {
		return errors.Annotate(err, "test")
	}
	return nil
}
`[1:],
	},
	// This test is checking that with no alias for the juju errors import the
	// error still gets nil checked correctly.
	{
		Name: "no alias for juju errors",
		Input: `
package main

import (
	"github.com/juju/errors"
)

func Test() error {
	err := errors.New("test")
	return errors.Annotate(err, "test")
}
`[1:],
		Expected: `
package main

import (
	"github.com/juju/errors"
)

func Test() error {
	err := errors.New("test")
	if err != nil {
		return errors.Annotate(err, "test")
	}
	return nil
}
`[1:],
	},
	// This test is checking that errors returned in closures are nil checked
	{
		Name: "error in closure not checked",
		Input: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() error {
	x := func() error {
		e := errors.New("some err")
		return errors.Annotate(e, "")
	}
	x()
	return nil
}
`[1:],
		Expected: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() error {
	x := func() error {
		e := errors.New("some err")
		if e != nil {
			return errors.Annotate(e, "")
		}
		return nil
	}
	x()
	return nil
}
`[1:],
	},
	// This test is checking an error that an error is not nil tested with
	// multiple return values.
	{
		Name: "error not checked with multiple returns",
		Input: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	return "test", 1 != 2, errors.Annotate(e, "")
}
`[1:],
		Expected: `
package main

import (
	errors "github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	if e != nil {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "test", 1 != 2, nil
}
`[1:],
	},
	// This test exists to test that when multiple return types exist and the
	// error being used is nil checked no changes are performed.
	{
		Name: "error is checked with multiple return values",
		Input: `
package main

import (
	"fmt"
	errors "github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	if e != nil {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	x := 1 + 2
	fmt.Println(x)
	return "", false, nil
}
`[1:],
		Expected: `
package main

import (
	"fmt"
	errors "github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	if e != nil {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	x := 1 + 2
	fmt.Println(x)
	return "", false, nil
}
`[1:],
	},
	// This test is making sure that even if the return is inside of an if
	// statement that has nothing to do with the error we still correctly add
	// the nil check.
	{
		Name: "error is checked when inside if statement",
		Input: `
package main

import (
	"github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	if 1 == 1 {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "foo", false, nil
}
`[1:],
		Expected: `
package main

import (
	"github.com/juju/errors"
)

func Test() (string, bool, error) {
	e := errors.New("some err")
	if 1 == 1 {
		if e != nil {
			return "test", 1 != 2, errors.Annotate(e, "")
		}
		return "test", 1 != 2, nil
	}
	return "foo", false, nil
}
`[1:],
	},
	// Test to make sure that when we get back an error from a func and don't
	// nil check it properly that it gets checked.
	{
		Name: "error is checked from func",
		Input: `
package main

import (
	"github.com/juju/errors"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	e := Error()
	return "test", 1 != 2, errors.Annotate(e, "")
}
`[1:],
		Expected: `
package main

import (
	"github.com/juju/errors"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	e := Error()
	if e != nil {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "test", 1 != 2, nil
}
`[1:],
	},
	// This test is testing spread import statements. This was a regression
	// found during development where we found that if there more errors to
	// process after finding juju/errors it would overwrite the errors alias
	// that had been established.
	{
		Name: "mixed import location",
		Input: `
package main

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	e := Error()
	return "test", 1 != 2, errors.Annotate(e, "")
}
`[1:],
		Expected: `
package main

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	e := Error()
	if e != nil {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "test", 1 != 2, nil
}
`[1:],
	},
	// Regression test to make sure that if we have an err != nil check mixed
	// with other conditions in an if statement we consider the err protected.
	// We don't want to evaluate these anymore as it generally shows that the
	// creator has put thought into the checks.
	{
		Name: "multiple if conditions to eval",
		Input: `
package main

import (
	"errors"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	testErr := errors.New("test")
	e := Error()
	if e != nil && !errors.Is(e, testErr) {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "test", false, nil
}
`[1:],
		Expected: `
package main

import (
	"errors"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	testErr := errors.New("test")
	e := Error()
	if e != nil && !errors.Is(e, testErr) {
		return "test", 1 != 2, errors.Annotate(e, "")
	}
	return "test", false, nil
}
`[1:],
	},
	{
		Name: "embedded functions that error",
		Input: `
package main

import (
	"errors"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	return "test", false, errors.Annotate(Error(), "something")
}
`[1:],
		Expected: `
package main

import (
	"errors"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/model/service"
)

func Error() error {
	return errors.New("some err")
}

func Test() (string, bool, error) {
	autoErr := Error()
	if autoErr != nil {
		return "test", false, errors.Annotate(autoErr, "something")
	}
	return "test", false, nil
}
`[1:],
	},
}

func (s *returnSuite) TestProcessFile(c *gc.C) {
	fset := token.NewFileSet()
	for i, test := range tests {
		c.Logf("Running test %d: %s", i, test.Name)
		testFile, err := parser.ParseFile(
			fset, "test", test.Input, parser.ParseComments,
		)
		c.Assert(err, jc.ErrorIsNil)
		processFile(testFile)
		c.Check(err, jc.ErrorIsNil)

		outputBuf := strings.Builder{}
		printer.Fprint(&outputBuf, fset, testFile)

		c.Check(outputBuf.String(), gc.Equals, test.Expected, gc.Commentf(outputBuf.String()))
	}
}
