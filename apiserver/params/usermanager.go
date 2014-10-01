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
	Deactivated    bool       `json:"deactivated"`
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
	Entities           []Entity `json:"tags"`
	IncludeDeactivated bool     `json:"include-deactivated"`
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

type AddUserResults struct {
	Results []AddUserResult `json:"result"`
}

type AddUserResult struct {
	Tag   string `json:"tag,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// DeactivateUsers holds the parameters for deactivating (or activating)
// users.
type DeactivateUsers struct {
	Users []DeactivateUser `json:"users"`
}

// DeactivateUser stores the parameters to deactivate or activate a single user.
type DeactivateUser struct {
	Tag        string `json:"tag"`
	Deactivate bool   `json:"deactivate"`
}
