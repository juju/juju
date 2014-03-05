// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"
)

type SimpleMessage struct {
	Level   loggo.Level
	Message string
}

type SimpleMessages []SimpleMessage

func (s SimpleMessage) String() string {
	return fmt.Sprintf("%s %s", s.Level, s.Message)
}

func (s SimpleMessages) GoString() string {
	out := make([]string, len(s))
	for i, m := range s {
		out[i] = m.String()
	}
	return fmt.Sprintf("SimpleMessages{\n%s\n}", strings.Join(out, "\n"))
}

func logToSimpleMessages(log []loggo.TestLogValues) SimpleMessages {
	out := make(SimpleMessages, len(log))
	for i, val := range log {
		out[i].Level = val.Level
		out[i].Message = val.Message
	}
	return out
}

type logMatches struct {
	*gc.CheckerInfo
}

func (checker *logMatches) Check(params []interface{}, names []string) (result bool, error string) {
	var obtained SimpleMessages
	switch params[0].(type) {
	case []loggo.TestLogValues:
		obtained = logToSimpleMessages(params[0].([]loggo.TestLogValues))
	default:
		return false, "Obtained value must be of type []loggo.TestLogValues or SimpleMessage"
	}

	var expected SimpleMessages
	switch param := params[1].(type) {
	case []SimpleMessage:
		expected = SimpleMessages(param)
	case SimpleMessages:
		expected = param
	case []string:
		expected = make(SimpleMessages, len(param))
		for i, s := range param {
			expected[i] = SimpleMessage{
				Message: s,
				Level:   loggo.UNSPECIFIED,
			}
		}
	default:
		return false, "Expected value must be of type []string or []SimpleMessage"
	}

	obtainedSinceLastMatch := obtained
	for len(expected) > 0 && len(obtained) >= len(expected) {
		var msg SimpleMessage
		msg, obtained = obtained[0], obtained[1:]
		expect := expected[0]
		if expect.Level != loggo.UNSPECIFIED && msg.Level != expect.Level {
			continue
		}
		matched, err := regexp.MatchString(expect.Message, msg.Message)
		if err != nil {
			return false, fmt.Sprintf("bad message regexp %q: %v", expect.Message, err)
		} else if !matched {
			continue
		}
		expected = expected[1:]
		obtainedSinceLastMatch = obtained
	}
	if len(obtained) < len(expected) {
		params[0] = obtainedSinceLastMatch
		params[1] = expected
		return false, ""
	}
	return true, ""
}

// LogMatches checks whether a given TestLogValues actually contains the log
// messages we expected. If you compare it against a list of strings, we only
// compare that the strings in the messages are correct. You can alternatively
// pass a slice of SimpleMessage and we will check that the log levels are
// also correct.
//
// The log may contain additional messages before and after each of the specified
// expected messages.
var LogMatches gc.Checker = &logMatches{
	&gc.CheckerInfo{Name: "LogMatches", Params: []string{"obtained", "expected"}},
}
