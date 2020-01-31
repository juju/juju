// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

var containerTemplate = `
- name: {{.Name}}
  {{if .Ports}}
  ports:
  {{- range .Ports }}
    - containerPort: {{.ContainerPort}}
      {{if .Name}}name: {{.Name}}{{end}}
      {{if .Protocol}}protocol: {{.Protocol}}{{end}}
  {{- end}}
  {{end}}
  {{if .Command}}
  command: 
{{ toYaml .Command | indent 4 }}
  {{end}}
  {{if .Args}}
  args: 
{{ toYaml .Args | indent 4 }}
  {{end}}
  {{if .WorkingDir}}
  workingDir: {{.WorkingDir}}
  {{end}}`[1:]

var defaultPodTemplateStr = fmt.Sprintf(`
{{if .Containers}}
containers:
{{- range .Containers }}
%s
{{- end}}
{{end}}
{{if .InitContainers}}
initContainers:
{{- range .InitContainers }}
%s
{{- end}}
{{end}}
`[1:], containerTemplate, containerTemplate)

var defaultPodTemplate = template.Must(template.New("").Funcs(templateAddons).Parse(defaultPodTemplateStr))

func toYaml(val interface{}) (string, error) {
	data, err := yaml.Marshal(val)
	if err != nil {
		return "", errors.Annotatef(err, "marshalling to yaml for %v", val)
	}
	return string(data), nil
}

func indent(n int, str string) string {
	out := ""
	prefix := strings.Repeat(" ", n)
	for _, line := range strings.Split(str, "\n") {
		out += prefix + line + "\n"
	}
	return out
}

var templateAddons = template.FuncMap{
	"toYaml": toYaml,
	"indent": indent,
}
