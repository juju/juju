// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"testing"

	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

func TestCleanupsHappenInReverse(t *testing.T) {
	var (
		firstCleanUpCalled  = false
		secondCleanUpCalled = false
	)

	utils.RunCleanUps([]func(){
		func() {
			firstCleanUpCalled = true
		},
		func() {
			secondCleanUpCalled = true
			if firstCleanUpCalled {
				t.Error("cleanup functions not called in reverse order")
			}
		},
	})

	if !firstCleanUpCalled || !secondCleanUpCalled {
		t.Error("not all cleanup functions called")
	}
}

func TestEmptyCleanUps(_ *testing.T) {
	utils.RunCleanUps([]func(){})
}
