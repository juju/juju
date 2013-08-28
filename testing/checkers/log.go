// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"reflect"

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
	switch params[1].(type) {
	case []SimpleMessage:
		return reflect.DeepEqual(obtained, params[1]), ""
	case []string:
		asString := make([]string, len(obtained))
		for i, val := range obtained {
			asString[i] = val.Message
		}
		return reflect.DeepEqual(asString, params[1]), ""
	default:
		return false, "Expected value must be of type []string or []SimpleMessage"
	}
}

// LogMatches checks whether a given TestLogValues actually contains the log
// messages we expected. If you compare it against a list of strings, we only
// compare that the strings in the messages are correct. You can alternatively
// pass a slice of SimpleMessage and we will check that the log levels are
// also correct.
var LogMatches gc.Checker = &logMatches{
	&gc.CheckerInfo{Name: "LogMatches", Params: []string{"obtained", "expected"}},
}
