// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Copied from yum_constants.go

package config

import (
	"text/template"
)

const (
	// ZypperSourcesDir is the default directory in which yum sourcefiles are located.
	ZypperSourcesDir = "/etc/zypp/repos.d"
)

// ZypperSourceTemplate is the template specific to a yum source file.
var ZypperSourceTemplate = template.Must(template.New("").Parse(`
[{{.Name}}]
name={{.Name}} (added by Juju)
baseurl={{.URL}}
{{if .Key}}gpgcheck=1
gpgkey=%s{{end}}
autorefresh=0
enabled=1
`[1:]))
