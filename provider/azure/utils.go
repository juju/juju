// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
)

const (
	retryDelay       = 5 * time.Second
	maxRetryDelay    = 1 * time.Minute
	maxRetryDuration = 5 * time.Minute
)

func toTags(tags *map[string]*string) map[string]string {
	if tags == nil {
		return nil
	}
	return to.StringMap(*tags)
}

// randomAdminPassword returns a random administrator password for
// Windows machines.
func randomAdminPassword() string {
	// We want at least one each of lower-alpha, upper-alpha, and digit.
	// Allocate 16 of each (randomly), and then the remaining characters
	// will be randomly chosen from the full set.
	validRunes := append(utils.LowerAlpha, utils.Digits...)
	validRunes = append(validRunes, utils.UpperAlpha...)

	lowerAlpha := utils.RandomString(16, utils.LowerAlpha)
	upperAlpha := utils.RandomString(16, utils.UpperAlpha)
	digits := utils.RandomString(16, utils.Digits)
	mixed := utils.RandomString(16, validRunes)
	password := []rune(lowerAlpha + upperAlpha + digits + mixed)
	for i := len(password) - 1; i >= 1; i-- {
		j := rand.Intn(i + 1)
		password[i], password[j] = password[j], password[i]
	}
	return string(password)
}

// callAPIFunc is a function type that should wrap any any
// Azure Resource Manager API calls.
type callAPIFunc func(func() (autorest.Response, error)) error

// backoffAPIRequestCaller is a type whose "call" method can
// be used as a callAPIFunc.
type backoffAPIRequestCaller struct {
	clock clock.Clock
}

// call will call the supplied function, with exponential backoff
// as long as the request returns an http.StatusTooManyRequests
// status.
func (c backoffAPIRequestCaller) call(f func() (autorest.Response, error)) error {
	var resp *http.Response
	return retry.Call(retry.CallArgs{
		Func: func() error {
			autorestResp, err := f()
			resp = autorestResp.Response
			return err
		},
		IsFatalError: func(err error) bool {
			return resp == nil || !autorest.ResponseHasStatusCode(resp, http.StatusTooManyRequests)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("attempt %d: %v", attempt, err)
		},
		Attempts:    -1,
		Delay:       retryDelay,
		MaxDelay:    maxRetryDelay,
		MaxDuration: maxRetryDuration,
		BackoffFunc: retry.DoubleDelay,
		Clock:       c.clock,
	})
}
