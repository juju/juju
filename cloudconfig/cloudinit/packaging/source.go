// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

import (
	"bytes"
	"text/template"
)

// Source contains all the data required of a package source.
type Source struct {
	Url   string              // name/url of the source
	Key   string              // key for the source, optional
	Prefs *PackagePreferences // preferences, optional
}

// AptPreferences is a set of apt_preferences(5) compatible preferences for an
// apt source. It can be used to override the default priority for the source.
// Path where the file will be created (usually in /etc/apt/preferences.d/).
type PackagePreferences struct {
	Path        string // the file the prefs will be written at
	Explanation string // a short explanation for the preference
	Package     string // the name of the package the preference applies to
	Pin         string // a pin on a certain source
	Priority    int    // the priority of that source
}

// FileContents returns contents of the package-manager specific config file
// of this paritcular package source.
func (s *PackagePreferences) FileContents() string {
	const prefTemplate = `
Explanation: {{.Explanation}}
Package: {{.Package}}
Pin: {{.Pin}}
Pin-Priority: {{.PinPriority}}
`

	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(prefTemplate[1:]))
	err := t.Execute(&buf, s)
	if err != nil {
		panic(err)
	}

	return buf.String()
}
