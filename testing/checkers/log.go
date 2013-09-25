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

func (checker *logMatches) Check(params []interface{}, names []string) (result bool, error string) {
	var obtained []SimpleMessage
	switch params[0].(type) {
	case []loggo.TestLogValues:
		obtained = logToSimpleMessages(params[0].([]loggo.TestLogValues))
	default:
		return false, "Obtained value must be of type []loggo.TestLogValues or SimpleMessage"
	}
	var expected []SimpleMessage
	checkLogLevel := true
	switch params[1].(type) {
	case []SimpleMessage:
		expected = params[1].([]SimpleMessage)
	case []string:
		checkLogLevel = false
		for _, message := range params[1].([]string) {
			expected = append(expected, SimpleMessage{Message: message})
		}
	default:
		return false, "Expected value must be of type []string or []SimpleMessage"
	}
	for lineNum := 0; ; lineNum++ {
		switch {
		case lineNum == len(obtained) && lineNum == len(expected):
			return true, ""
		case lineNum == len(obtained):
			names[0] = fmt.Sprintf("too few log messages found, <missing line %d>", lineNum)
			params[0] = nil
			names[1] = "expected regex"
			params[1] = expected[lineNum].Message
			return false, ""
		case lineNum == len(expected):
			names[0] = "too many log messages found, got"
			params[0] = obtained[lineNum].Message
			names[1] = fmt.Sprintf("<no expected line %d>", lineNum)
			params[1] = nil
			return false, ""
		}
		if checkLogLevel {
			obtainedLevel := obtained[lineNum].Level
			expectedLevel := expected[lineNum].Level
			if obtainedLevel != expectedLevel {
				names[0] = fmt.Sprintf("line %d obtained level", lineNum)
				params[0] = obtainedLevel
				names[1] = fmt.Sprintf("line %d expected level", lineNum)
				params[1] = expectedLevel
				return false, ""
			}
		}
		expectedRegex := expected[lineNum].Message
		obtainedMessage := obtained[lineNum].Message
		re := regexp.MustCompile(expectedRegex)
		if !re.MatchString(obtainedMessage) {
			names[0] = fmt.Sprintf("line %d obtained message", lineNum)
			params[0] = obtainedMessage
			names[1] = fmt.Sprintf("line %d expected regex", lineNum)
			params[1] = expectedRegex
			return false, ""
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
