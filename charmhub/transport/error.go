// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"strings"
)

// APIError represents the error from the CharmHub API.
type APIError struct {
	Code    APIErrorCode  `json:"code"`
	Message string        `json:"message"`
	Extra   APIErrorExtra `json:"extra"`
}

func (a APIError) Error() string {
	return a.Message
}

// APIErrors represents a slice of APIError's
type APIErrors []APIError

func (a APIErrors) Error() string {
	if len(a) > 0 {
		var combined []string
		for _, e := range a {
			if err := e.Error(); err != "" {
				combined = append(combined, err)
			}
		}
		return strings.Join(combined, "\n")
	}
	return ""
}

// APIErrorExtra defines additional extra payloads from a given error. Think
// of this object as a series of suggestions to perform against the errorred
// API request, in the chance of the new request being successful.
type APIErrorExtra struct {
	Releases     []Release `json:"releases"`
	DefaultBases []Base    `json:"default-bases"`
}

// Release defines a set of suggested releases that might also work for the
// given request.
type Release struct {
	Base    Base   `json:"base"`
	Channel string `json:"channel"`
}

// APIErrorCode classifies the error code we get back from the API. This isn't
// tautological list of codes.
type APIErrorCode string

const (
	ErrorCodeAccessByDownstreamStoreNotAllowed APIErrorCode = "access-by-downstream-store-not-allowed"
	ErrorCodeAccessByRevisionNotAllowed        APIErrorCode = "access-by-revision-not-allowed"
	ErrorCodeAPIError                          APIErrorCode = "api-error"
	ErrorCodeBadArgument                       APIErrorCode = "bad-argument"
	ErrorCodeCharmResourceNotFound             APIErrorCode = "charm-resource-not-found"
	ErrorCodeChannelNotFound                   APIErrorCode = "channel-not-found"
	ErrorCodeDeviceAuthorizationNeedsRefresh   APIErrorCode = "device-authorization-needs-refresh"
	ErrorCodeDeviceServiceDisallowed           APIErrorCode = "device-service-disallowed"
	ErrorCodeDuplicatedKey                     APIErrorCode = "duplicated-key"
	ErrorCodeDuplicateFetchAssertionsKey       APIErrorCode = "duplicate-fetch-assertions-key"
	ErrorCodeEndpointDisabled                  APIErrorCode = "endpoint-disabled"
	ErrorCodeIDNotFound                        APIErrorCode = "id-not-found"
	ErrorCodeInconsistentData                  APIErrorCode = "inconsistent-data"
	ErrorCodeInstanceKeyNotUnique              APIErrorCode = "instance-key-not-unique"
	ErrorCodeInvalidChannel                    APIErrorCode = "invalid-channel"
	ErrorCodeInvalidCharmBase                  APIErrorCode = "invalid-charm-base"
	ErrorCodeInvalidCharmResource              APIErrorCode = "invalid-charm-resource"
	ErrorCodeInvalidCohortKey                  APIErrorCode = "invalid-cohort-key"
	ErrorCodeInvalidGrade                      APIErrorCode = "invalid-grade"
	ErrorCodeInvalidMetric                     APIErrorCode = "invalid-metric"
	ErrorCodeInvalidUnboundEmptySearch         APIErrorCode = "invalid-unbound-empty-search"
	ErrorCodeMacaroonPermissionRequired        APIErrorCode = "macaroon-permission-required"
	ErrorCodeMissingCharmBase                  APIErrorCode = "missing-charm-base"
	ErrorCodeMissingContext                    APIErrorCode = "missing-context"
	ErrorCodeMissingFetchAssertionsKey         APIErrorCode = "missing-fetch-assertions-key"
	ErrorCodeMissingHeader                     APIErrorCode = "missing-header"
	ErrorCodeMissingInstanceKey                APIErrorCode = "missing-instance-key"
	ErrorCodeMissingKey                        APIErrorCode = "missing-key"
	ErrorCodeNameNotFound                      APIErrorCode = "name-not-found"
	ErrorCodeNotFound                          APIErrorCode = "not-found"
	ErrorCodePaymentRequired                   APIErrorCode = "payment-required"
	ErrorCodeRateLimitExceeded                 APIErrorCode = "rate-limit-exceeded"
	ErrorCodeRefreshBundleNotSupported         APIErrorCode = "refresh-bundle-not-supported"
	ErrorCodeRemoteServiceUnavailable          APIErrorCode = "remote-service-unavailable"
	ErrorCodeResourceNotFound                  APIErrorCode = "resource-not-found"
	ErrorCodeRevisionConflict                  APIErrorCode = "revision-conflict"
	ErrorCodeRevisionNotFound                  APIErrorCode = "revision-not-found"
	ErrorCodeServiceMisconfigured              APIErrorCode = "service-misconfigured"
	ErrorCodeStoreAuthorizationNeedsRefresh    APIErrorCode = "store-authorization-needs-refresh"
	ErrorCodeStoreDisallowed                   APIErrorCode = "store-disallowed"
	ErrorCodeUnexpectedData                    APIErrorCode = "unexpected-data"
	ErrorCodeUnknownGrade                      APIErrorCode = "unknown-grade"
	ErrorCodeUserAuthenticationError           APIErrorCode = "user-authentication-error"
	ErrorCodeUserAuthorizationNeedsRefresh     APIErrorCode = "user-authorization-needs-refresh"
	// TODO 2021-04-08 hml
	// Remove once Charmhub API returns ErrorCodeInvalidCharmBase
	ErrorCodeInvalidCharmPlatform APIErrorCode = "invalid-charm-platform"
	ErrorCodeMissingCharmPlatform APIErrorCode = "missing-charm-platform"
)
