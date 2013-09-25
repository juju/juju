// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"regexp"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"
)

type SimpleMessage struct {
	Level   loggo.Level
	Message string
}

const NoLevel = loggo.Level(0xFFFFFFFF)

func logToSimpleMessages(log []loggo.TestLogValues) []SimpleMessage {
	out := make([]SimpleMessage, len(log))
	for i, val := range log {
		out[i].Level = val.Level
		out[i].Message = val.Message
	}
	return out
}

type logMatches struct {
	*gc.CheckerInfo
}

// compareOne line of log output, return the empty string if it matches, a message otherwise
func compareOne(lineNum int, obtained, expected SimpleMessage) string {
	// We use -1 to indicate we don't have an expected log level
	if expected.Level != NoLevel {
		if obtained.Level != expected.Level {
			return fmt.Sprintf("on line %d got log level %d expected %d", lineNum, obtained.Level, expected.Level)
		}
	}
	re := regexp.MustCompile(expected.Message)
	if !re.MatchString(obtained.Message) {
		return fmt.Sprintf("on line %d\ngot %q\nexpected regex: %q", lineNum, obtained.Message, expected.Message)
	}
	return ""
}

func (checker *logMatches) Check(params []interface{}, names []string) (result bool, error string) {
	var obtained []SimpleMessage
	switch params[0].(type) {
	case []loggo.TestLogValues:
		obtained = logToSimpleMessages(params[0].([]loggo.TestLogValues))
	default:
		return false, "Obtained value must be of type []loggo.TestLogValues or SimpleMessage"
	}
	var expected []SimpleMessage
	switch params[1].(type) {
	case []SimpleMessage:
		expected = params[1].([]SimpleMessage)
	case []string:
		for _, message := range params[1].([]string) {
			expected = append(expected, SimpleMessage{Level: NoLevel, Message: message})
		}
	default:
		return false, "Expected value must be of type []string or []SimpleMessage"
	}
	for lineNum := 0; ; lineNum++ {
		switch {
		case lineNum == len(obtained) && lineNum == len(expected):
			return true, ""
		case lineNum == len(obtained):
			return false, fmt.Sprintf("too few log messages found (expected %q at line %d)", expected[lineNum].Message, lineNum)
		case lineNum == len(expected):
			return false, fmt.Sprintf("too many log messages found (got %q at line %d)", obtained[lineNum].Message, lineNum)
		}
		mismatchMessage := compareOne(lineNum, obtained[lineNum], expected[lineNum])
		if mismatchMessage != "" {
			return false, mismatchMessage
		}
	}
	return true, ""
}

// LogMatches checks whether a given TestLogValues actually contains the log
// messages we expected. If you compare it against a list of strings, we only
// compare that the strings in the messages are correct. You can alternatively
// pass a slice of SimpleMessage and we will check that the log levels are
// also correct.
var LogMatches gc.Checker = &logMatches{
	&gc.CheckerInfo{Name: "LogMatches", Params: []string{"obtained", "expected"}},
}
