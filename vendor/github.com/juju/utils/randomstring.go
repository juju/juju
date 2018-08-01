// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"math/rand"
	"sync"
	"time"
)

// Can be used as a sane default argument for RandomString
var (
	LowerAlpha = []rune("abcdefghijklmnopqrstuvwxyz")
	UpperAlpha = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	Digits     = []rune("0123456789")
)

var (
	randomStringMu   sync.Mutex
	randomStringRand *rand.Rand
)

func init() {
	randomStringRand = rand.New(
		rand.NewSource(time.Now().UnixNano()),
	)
}

// RandomString will return a string of length n that will only
// contain runes inside validRunes
func RandomString(n int, validRunes []rune) string {
	randomStringMu.Lock()
	defer randomStringMu.Unlock()

	runes := make([]rune, n)
	for i := range runes {
		runes[i] = validRunes[randomStringRand.Intn(len(validRunes))]
	}

	return string(runes)
}
