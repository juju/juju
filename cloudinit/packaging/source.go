// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

import (
	"bytes"
	"text/template"
)

// Source contains all the data required of a package source
type Source struct {
	Name     string // name of the source
	Source   string // source URL
	Key      string // key for the source, optional
	template string // apt/yum specific template for the source file
}

// FileContents returns contents of the package-manager specific config file
// of this paritcular package source
func (s *Source) FileContents() string {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(s.template))

	err := t.Execute(&buf, s)
	if err != nil {
		panic(err)
	}

	return buf.String()
}
