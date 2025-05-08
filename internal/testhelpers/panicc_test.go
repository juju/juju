// Copyright 2025 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/juju/tc"
)

type panicC struct {
	failed    atomic.Bool
	errString string
}

func (pc *panicC) recover() {
	if err := recover(); err != nil {
		if !pc.failed.Swap(true) {
			pc.errString = fmt.Sprintf("%v", err)
		}
	}
}

func (pc *panicC) Assert(obtained any, checker tc.Checker, args ...any) {
	params := append([]any{obtained}, args...)
	ok, errString := checker.Check(params, slices.Clone(checker.Info().Params))
	if !ok {
		panic(errString)
	}
}

func (pc *panicC) Check(obtained any, checker tc.Checker, args ...any) bool {
	params := append([]any{obtained}, args...)
	ok, errString := checker.Check(params, slices.Clone(checker.Info().Params))
	if !ok {
		if !pc.failed.Swap(true) {
			pc.errString = errString
		}
	}
	return ok
}
