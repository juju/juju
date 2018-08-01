// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

import "regexp"

func equalTopic(match string) func(topic string) bool {
	return func(topic string) bool {
		return match == topic
	}
}

// MatchRegexp expects a valid regular expression. If the expression
// passed in is not valid, the function panics. The expected use of this
// is to be able to do something like:
//
//     hub.SubscribeMatch(pubsub.MatchRegex("prefix.*suffix"), handler)
func MatchRegexp(expression string) func(topic string) bool {
	r := regexp.MustCompile(expression)
	return func(topic string) bool {
		return r.MatchString(string(topic))
	}
}

// MatchAll is a topic matcher that matches all topics.
func MatchAll(_ string) bool {
	return true
}
