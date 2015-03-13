// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

import (
	"bytes"
	"fmt"
	"text/template"
)

// Source contains all the data required of a package source.
type Source struct {
	Name string `yaml:"-"`             // name of the source
	Url  string `yaml:"source"`        // url of the source
	Key  string `yaml:"key,omitempty"` // key for the source, optional
}

// KeyfileName returns the path of the gpg keyfile associated to this source.
func (s *Source) KeyfilePath() string {
	return CentOSYumKeyfileDir + s.Name + ".key"
}

// RenderCentOS returns the source rendered as a yum sourcefile.
func (s *Source) RenderCentOS() string {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(YumSourceTemplate[1:]))

	if err := t.Execute(&buf, s); err != nil {
		panic(err)
	}
	contents := buf.String()

	// check if gpg key required and add path to keyfile if so.
	if s.Key != "" {
		contents = fmt.Sprintf(contents, "file://"+s.KeyfilePath())
	}

	return contents
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
	var buf bytes.Buffer

	t := template.Must(template.New("").Parse(AptPreferenceTemplate[1:]))
	err := t.Execute(&buf, s)
	if err != nil {
		panic(err)
	}

	return buf.String()
}
