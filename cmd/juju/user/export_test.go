// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

var (
	RandomPasswordNotify = &randomPasswordNotify
	ReadPassword         = &readPassword
	ServerFileNotify     = &serverFileNotify
	WriteServerFile      = writeServerFile
)

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommand(api AddUserAPI) *AddCommand {
	return &AddCommand{
		api: api,
	}
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommand(api ChangePasswordAPI, writer EnvironInfoCredsWriter) *ChangePasswordCommand {
	return &ChangePasswordCommand{
		api:    api,
		writer: writer,
	}
}

// NewDisableCommand returns a DisableCommand with the api provided as
// specified.
func NewDisableCommand(api DisenableUserAPI) *DisableCommand {
	return &DisableCommand{
		DisenableUserBase{
			api: api,
		},
	}
}

// NewEnableCommand returns a EnableCommand with the api provided as
// specified.
func NewEnableCommand(api DisenableUserAPI) *EnableCommand {
	return &EnableCommand{
		DisenableUserBase{
			api: api,
		},
	}
}

// NewInfoCommand returns an InfoCommand with the api provided as specified.
func NewInfoCommand(api UserInfoAPI) *InfoCommand {
	return &InfoCommand{
		InfoCommandBase: InfoCommandBase{
			api: api,
		},
	}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommand(api UserInfoAPI) *ListCommand {
	return &ListCommand{
		InfoCommandBase: InfoCommandBase{
			api: api,
		},
	}
}
