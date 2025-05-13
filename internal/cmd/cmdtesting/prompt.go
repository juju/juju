// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmdtesting

import (
	"context"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.cmd.testing")

// NewSeqPrompter returns a prompter that can be used to check a sequence of
// IO interactions. Expected input from the user is marked with the
// given user input marker (for example a distinctive unicode character
// that will not occur in the rest of the text) and runs to the end of a
// line.
//
// All output text in between user input is treated as regular expressions.
//
// As a special case, if an input marker is followed only by a single input
// marker on that line, the checker will cause io.EOF to be returned for
// that prompt.
//
// The returned SeqPrompter wraps a Prompter and checks that each
// read and write corresponds to the expected action in the sequence.
//
// After all interaction is done, CheckDone or AssertDone should be called to
// check that no more interactions are expected.
//
// Any failures will result in the test failing.
//
// For example given the prompter created with:
//
//		checker := NewSeqPrompter(c, "»",  `What is your name: »Bob
//	And your age: »148
//	You're .* old, Bob!
//	`)
//
// The following code will pass the checker:
//
//	fmt.Fprintf(checker, "What is your name: ")
//	buf := make([]byte, 100)
//	n, _ := checker.Read(buf)
//	name := strings.TrimSpace(string(buf[0:n]))
//	fmt.Fprintf(checker, "And your age: ")
//	n, _ = checker.Read(buf)
//	age, err := strconv.Atoi(strings.TrimSpace(string(buf[0:n])))
//	c.Assert(err, tc.IsNil)
//	if age > 90 {
//		fmt.Fprintf(checker, "You're very old, %s!\n", name)
//	}
//	checker.CheckDone()
func NewSeqPrompter(c *tc.C, userInputMarker, text string) *SeqPrompter {
	p := &SeqPrompter{
		c: c,
	}
	for {
		i := strings.Index(text, userInputMarker)
		if i == -1 {
			p.finalText = text
			break
		}
		prompt := text[0:i]
		text = text[i+len(userInputMarker):]
		endLine := strings.Index(text, "\n")
		if endLine == -1 {
			c.Errorf("no newline found after expected input %q", text)
		}
		reply := text[0 : endLine+1]
		if reply[0:len(reply)-1] == userInputMarker {
			// EOF line.
			reply = ""
		}
		text = text[endLine+1:]
		if prompt == "" && len(p.ios) > 0 {
			// Combine multiple contiguous inputs together.
			p.ios[len(p.ios)-1].reply += reply
		} else {
			p.ios = append(p.ios, ioInteraction{
				prompt: prompt,
				reply:  reply,
			})
		}
	}
	p.Prompter = NewPrompter(p.prompt)
	return p
}

type SeqPrompter struct {
	*Prompter
	c         *tc.C
	ios       []ioInteraction
	finalText string
	failed    bool
}

type ioInteraction struct {
	prompt string
	reply  string
}

func (p *SeqPrompter) prompt(text string) (string, error) {
	if p.failed {
		return "", errors.New("prompter failed")
	}
	if len(p.ios) == 0 {
		p.c.Errorf("unexpected prompt %q; expected none", text)
		return "", errors.New("unexpected prompt")
	}
	if !p.c.Check(text, tc.Matches, p.ios[0].prompt) {
		p.failed = true
		return "", errors.Errorf("unexpected prompt %q; expected %q", text, p.ios[0].prompt)
	}
	reply := p.ios[0].reply
	logger.Infof(context.TODO(), "prompt %q -> %q", text, reply)
	p.ios = p.ios[1:]
	return reply, nil
}

// CheckDone asserts that all the expected prompts
// have been printed and all the replies read, and
// reports whether the check succeeded.
func (p *SeqPrompter) CheckDone() bool {
	if p.failed {
		// No point in doing the details checks if
		// a prompt failed earlier - it just makes
		// the resulting test failure noisy.
		p.c.Errorf("prompter has failed")
		return false
	}
	r := p.c.Check(p.ios, tc.HasLen, 0, tc.Commentf("unused prompts"))
	r = p.c.Check(p.HasUnread(), tc.Equals, false, tc.Commentf("some input was not read")) && r
	r = p.c.Check(p.Tail(), tc.Matches, p.finalText, tc.Commentf("final text mismatch")) && r
	return r
}

// AssertDone is like CheckDone but aborts the test if
// the check fails.
func (p *SeqPrompter) AssertDone() {
	if !p.CheckDone() {
		p.c.FailNow()
	}
}

// NewPrompter returns an io.ReadWriter implementation that calls the
// given function every time Read is called after some text has been
// written or if all the previously returned text has been read. The
// function's argument contains all the text printed since the last
// input. The function should return the text that the user is expected
// to type, or an error to return from Read. If it returns an empty string,
// and no error, it will return io.EOF instead.
func NewPrompter(prompt func(string) (string, error)) *Prompter {
	return &Prompter{
		prompt: prompt,
	}
}

// Prompter is designed to be used in a cmd.Context to
// check interactive request-response sequences
// using stdin and stdout.
type Prompter struct {
	prompt func(string) (string, error)

	written      []byte
	allWritten   []byte
	pending      []byte
	pendingError error
}

// Tail returns all the text written since the last prompt.
func (p *Prompter) Tail() string {
	return string(p.written)
}

// HasUnread reports whether any input
// from the last prompt remains unread.
func (p *Prompter) HasUnread() bool {
	return len(p.pending) != 0
}

// Read implements io.Reader.
func (p *Prompter) Read(buf []byte) (int, error) {
	if len(p.pending) == 0 && p.pendingError == nil {
		s, err := p.prompt(string(p.written))
		if s == "" && err == nil {
			err = io.EOF
		}
		p.written = nil
		p.pending = []byte(s)
		p.pendingError = err
	}
	if len(p.pending) > 0 {
		n := copy(buf, p.pending)
		p.pending = p.pending[n:]
		return n, nil
	}
	if err := p.pendingError; err != nil {
		p.pendingError = nil
		return 0, err
	}
	panic("unreachable")
}

// String returns all the text that has been written to
// the prompter since it was created.
func (p *Prompter) String() string {
	return string(p.allWritten)
}

// Write implements io.Writer.
func (p *Prompter) Write(buf []byte) (int, error) {
	p.written = append(p.written, buf...)
	p.allWritten = append(p.allWritten, buf...)
	return len(buf), nil
}
