// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package securitylog

import (
	"testing"
)

// TestFormLogUserSecurityEvent tests the formLogUserSecurityEvent function.
func TestFormLogUserSecurityEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    UserSecurityEvent
		wantDesc string
		wantEvt  string
	}{
		{
			name:     "user created with login access",
			event:    UserSecurityEvent{Actor: "admin", Target: "newuser", Action: "created", Access: "login", AppID: "juju.controller"},
			wantDesc: "User admin created user newuser with login access level",
			wantEvt:  "user_created:admin,newuser:login",
		},
		{
			name:     "user deleted with admin access",
			event:    UserSecurityEvent{Actor: "admin", Target: "olduser", Action: "deleted", Access: "admin", AppID: "juju.controller"},
			wantDesc: "User admin deleted user olduser with admin access level",
			wantEvt:  "user_deleted:admin,olduser:admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt := formLogUserSecurityEvent(tt.event)
			if gotEvt.Description != tt.wantDesc {
				t.Errorf("formLogUserSecurityEvent() gotDesc = %v, want %v", gotEvt.Description, tt.wantDesc)
			}
			if gotEvt.Event != tt.wantEvt {
				t.Errorf("formLogUserSecurityEvent() gotEvt = %v, want %v", gotEvt.Event, tt.wantEvt)
			}
		})
	}
}

// TestFormLogAuthzSecurityEvent tests the formLogAuthzSecurityEvent function.
func TestFormLogAuthzSecurityEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    AuthzSecurityEvent
		wantDesc string
		wantEvt  string
	}{
		{
			name:     "privilege granted",
			event:    AuthzSecurityEvent{Actor: "admin", Target: "user1", Action: "grant", NewLevel: "admin", AppID: "juju.controller"},
			wantDesc: "Administrator admin has updated privileges of user user1 to admin",
			wantEvt:  "authz_admin:admin,grant",
		},
		{
			name:     "privilege revoked",
			event:    AuthzSecurityEvent{Actor: "admin", Target: "user2", Action: "revoke", NewLevel: "login", AppID: "juju.controller"},
			wantDesc: "Administrator admin has revoked login privileges of user user2",
			wantEvt:  "authz_admin:admin,revoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt := formLogAuthzSecurityEvent(tt.event)
			if gotEvt.Description != tt.wantDesc {
				t.Errorf("formLogAuthzSecurityEvent() gotDesc = %v, want %v", gotEvt.Description, tt.wantDesc)
			}
			if gotEvt.Event != tt.wantEvt {
				t.Errorf("formLogAuthzSecurityEvent() gotEvt = %v, want %v", gotEvt.Event, tt.wantEvt)
			}
		})
	}
}

// TestFormPasswordChangeSecurityEvent tests the formPasswordChangeSecurityEvent function.
func TestFormPasswordChangeSecurityEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    PasswordChangeSecurityEvent
		wantDesc string
		wantEvt  string
	}{
		{
			name:     "password changed by user",
			event:    PasswordChangeSecurityEvent{User: "user1", AppID: "juju.controller"},
			wantDesc: "User user1 has successfully changed their password",
			wantEvt:  "authn_password_change:user1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt := formPasswordChangeSecurityEvent(tt.event)
			if gotEvt.Description != tt.wantDesc {
				t.Errorf("formPasswordChangeSecurityEvent() gotDesc = %v, want %v", gotEvt.Description, tt.wantDesc)
			}
			if gotEvt.Event != tt.wantEvt {
				t.Errorf("formPasswordChangeSecurityEvent() gotEvt = %v, want %v", gotEvt.Event, tt.wantEvt)
			}
		})
	}
}

// TestFormLoginSuccessSecurityEvent tests the formLoginSuccessSecurityEvent function.
func TestFormLoginSuccessSecurityEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    LoginSuccessSecurityEvent
		wantDesc string
		wantEvt  string
	}{
		{
			name:     "successful login",
			event:    LoginSuccessSecurityEvent{User: "user1", AppID: "juju.controller"},
			wantDesc: "User user1 has successfully logged in",
			wantEvt:  "authn_login_success:user1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt := formLoginSuccessSecurityEvent(tt.event)
			if gotEvt.Description != tt.wantDesc {
				t.Errorf("formLoginSuccessSecurityEvent() gotDesc = %v, want %v", gotEvt.Description, tt.wantDesc)
			}
			if gotEvt.Event != tt.wantEvt {
				t.Errorf("formLoginSuccessSecurityEvent() gotEvt = %v, want %v", gotEvt.Event, tt.wantEvt)
			}
		})
	}
}

// TestFormSystemLifecycleSecurityEvent tests the formSystemLifecycleSecurityEvent function.
func TestFormSystemLifecycleSecurityEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    SystemLifecycleSecurityEvent
		wantDesc string
		wantEvt  string
	}{
		{
			name:     "system startup",
			event:    SystemLifecycleSecurityEvent{Actor: "admin", Event: "startup", AppID: "juju.controller"},
			wantDesc: "User admin spawned a new instance",
			wantEvt:  "sys_startup:admin",
		},
		{
			name:     "system shutdown",
			event:    SystemLifecycleSecurityEvent{Actor: "admin", Event: "shutdown", AppID: "juju.controller"},
			wantDesc: "User admin stopped this instance",
			wantEvt:  "sys_shutdown:admin",
		},
		{
			name:     "system crash",
			event:    SystemLifecycleSecurityEvent{Event: "crash", AppID: "juju.controller"},
			wantDesc: "The system crashed due to unknown_reason",
			wantEvt:  "sys_crash:unknown_reason",
		},
		{
			name:     "restart",
			event:    SystemLifecycleSecurityEvent{Actor: "admin", Event: "restart", AppID: "juju.controller"},
			wantDesc: "User admin initiated a restart",
			wantEvt:  "sys_restart:admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt := formSystemLifecycleSecurityEvent(tt.event)
			if gotEvt.Description != tt.wantDesc {
				t.Errorf("formSystemLifecycleSecurityEvent() gotDesc = %v, want %v", gotEvt.Description, tt.wantDesc)
			}
			if gotEvt.Event != tt.wantEvt {
				t.Errorf("formSystemLifecycleSecurityEvent() gotEvt = %v, want %v", gotEvt.Event, tt.wantEvt)
			}
		})
	}
}
