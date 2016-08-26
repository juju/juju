// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agree

// These two var are exported becuase they are useful in tests outside of this
// package. Unless you are writing a test you shouldn't be using either of these
// values.
var (
	ClientNew  = &clientNew
	UserAnswer = &userAnswer
)
