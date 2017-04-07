// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmdtesting_test

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/cmdtesting"
)

type prompterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&prompterSuite{})

func (*prompterSuite) TestPrompter(c *gc.C) {
	noPrompt := func(p string) (string, error) {
		c.Fatalf("unpexected prompt (text %q)", p)
		panic("unreachable")
	}
	promptFn := noPrompt
	p := cmdtesting.NewPrompter(func(p string) (string, error) {
		return promptFn(p)
	})

	promptText := "hello: "
	promptReply := "reply\n"

	fmt.Fprint(p, promptText)
	promptFn = func(p string) (string, error) {
		c.Assert(p, gc.Equals, promptText)
		return promptReply, nil
	}
	c.Assert(readStr(c, p, 20), gc.Equals, promptReply)

	promptText = "some text\ngoodbye: "
	promptReply = "again\n"
	fmt.Fprint(p, promptText[0:10])
	fmt.Fprint(p, promptText[10:])

	c.Assert(readStr(c, p, 3), gc.Equals, promptReply[0:3])
	c.Assert(readStr(c, p, 20), gc.Equals, promptReply[3:])

	fmt.Fprint(p, "final text\n")

	c.Assert(p.Tail(), gc.Equals, "final text\n")
	c.Assert(p.HasUnread(), gc.Equals, false)
}

func (*prompterSuite) TestUnreadInput(c *gc.C) {
	p := cmdtesting.NewPrompter(func(s string) (string, error) {
		return "hello world", nil
	})
	c.Assert(readStr(c, p, 3), gc.Equals, "hel")

	c.Assert(p.HasUnread(), gc.Equals, true)
}

func (*prompterSuite) TestError(c *gc.C) {
	expectErr := errors.New("something")
	p := cmdtesting.NewPrompter(func(s string) (string, error) {
		return "", expectErr
	})
	buf := make([]byte, 3)
	n, err := p.Read(buf)
	c.Assert(n, gc.Equals, 0)
	c.Assert(err, gc.Equals, expectErr)
}

func (*prompterSuite) TestSeqPrompter(c *gc.C) {
	p := cmdtesting.NewSeqPrompter(c, "»", `
hello: »reply
some text
goodbye: »again
final
`[1:])
	fmt.Fprint(p, "hello: ")
	c.Assert(readStr(c, p, 1), gc.Equals, "r")
	c.Assert(readStr(c, p, 20), gc.Equals, "eply\n")
	fmt.Fprint(p, "some text\n")
	fmt.Fprint(p, "goodbye: ")
	c.Assert(readStr(c, p, 20), gc.Equals, "again\n")
	fmt.Fprint(p, "final\n")
	p.AssertDone()
}

func (*prompterSuite) TestSeqPrompterEOF(c *gc.C) {
	p := cmdtesting.NewSeqPrompter(c, "»", `
hello: »»
final
`[1:])
	fmt.Fprint(p, "hello: ")
	n, err := p.Read(make([]byte, 10))
	c.Assert(n, gc.Equals, 0)
	c.Assert(err, gc.Equals, io.EOF)
	fmt.Fprint(p, "final\n")
	p.AssertDone()
}

func (*prompterSuite) TestNewIOChecker(c *gc.C) {
	checker := cmdtesting.NewSeqPrompter(c, "»", `What is your name: »Bob
»more
And your age: »148
You're .* old, Bob
more!
`)
	fmt.Fprintf(checker, "What is your name: ")
	buf := make([]byte, 100)
	n, _ := checker.Read(buf)
	name := strings.TrimSpace(string(buf[0:n]))
	fmt.Fprintf(checker, "And your age: ")
	n, _ = checker.Read(buf)
	age, err := strconv.Atoi(strings.TrimSpace(string(buf[0:n])))
	c.Assert(err, gc.IsNil)
	if age > 90 {
		fmt.Fprintf(checker, "You're very old, %s!\n", name)
	}
	checker.CheckDone()
}

func readStr(c *gc.C, r io.Reader, nb int) string {
	buf := make([]byte, nb)
	n, err := r.Read(buf)
	c.Assert(err, jc.ErrorIsNil)
	return string(buf[0:n])
}
