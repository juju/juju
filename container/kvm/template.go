// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"bytes"
	"os"
	"text/template"

	"github.com/juju/errors"
)

var kvmTemplate = `
<domain type='kvm'>
  <name>{{.Hostname}}</name>
  <vcpu placement='static'>1</vcpu>
  <os>
    <type>hvm</type>
  </os>
  <features>
    <acpi/>
    <apic/>
    <pae/>
  </features>
  <devices>
    <controller type='usb' index='0'>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x01' function='0x2'/>
    </controller>
    <controller type='pci' index='0' model='pci-root'/>
    <serial type='stdio'>
      <target port='0'/>
    </serial>
    <console type='stdio'>
      <target type='serial' port='0'/>
    </console>
    <input type='mouse' bus='ps2'/>
    <input type='keyboard' bus='ps2'/>
    <graphics type='vnc' port='-1' autoport='yes' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>
    <video>
      <model type='cirrus' vram='9216' heads='1'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x02' function='0x0'/>
    </video>

    {{$bridge := .NetworkBridge}}{{range $nic := .Interfaces}}
    <interface type='bridge'>
      <mac address='{{$nic.MACAddress}}'/>
      <model type='virtio'/>
      <source bridge='{{$bridge}}'/>
    </interface>
    {{end}}
  </devices>
</domain>
`

func WriteTemplate(path string, params CreateMachineParams) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot write kvm container config")

	tmpl, err := template.New("kvm").Parse(kvmTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(buf.String())
	return err
}
