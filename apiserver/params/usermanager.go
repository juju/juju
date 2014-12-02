// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// UserInfo holds information on a user.
type UserInfo struct {
	Username       string     `json:"username"`
	DisplayName    string     `json:"display-name"`
	CreatedBy      string     `json:"created-by"`
	DateCreated    time.Time  `json:"date-created"`
	LastConnection *time.Time `json:"last-connection,omitempty"`
	Disabled       bool       `json:"disabled"`
}

// UserInfoResult holds the result of a UserInfo call.
type UserInfoResult struct {
	Result *UserInfo `json:"result,omitempty"`
	Error  *Error    `json:"error,omitempty"`
}

// UserInfoResults holds the result of a bulk UserInfo API call.
type UserInfoResults struct {
	Results []UserInfoResult `json:"results"`
}

// UserInfoRequest defines the users to return.  An empty
// Entities list indicates that all matching users should be returned.
type UserInfoRequest struct {
	Entities        []Entity `json:"entities"`
	IncludeDisabled bool     `json:"include-disabled"`
}

// AddUsers holds the parameters for adding new users.
type AddUsers struct {
	Users []AddUser `json:"users"`
}

// AddUser stores the parameters to add one user.
type AddUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display-name"`
	Password    string `json:"password"`
}

// AddUserResults holds the results of the bulk AddUser API call.
type AddUserResults struct {
	Results []AddUserResult `json:"results"`
}

// AddUserResult returns the tag of the newly created user, or an error.
type AddUserResult struct {
	Tag   string `json:"tag,omitempty"`
	Error *Error `json:"error,omitempty"`
}
