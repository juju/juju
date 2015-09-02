// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/daemon/network"
	"github.com/docker/docker/pkg/nat"
	"github.com/docker/docker/runconfig"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-process-docker/docker"
)

var _ = gc.Suite(&infoSuite{})

type infoSuite struct{}

func (infoSuite) TestParseInfoJSONOkay(c *gc.C) {
	info, err := docker.ParseInfoJSON("id", []byte(fakeInspectOutput))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, (*docker.Info)(fakeInfo))
}

func (infoSuite) TestParseInfoJSONNone(c *gc.C) {
	b := []byte("not json")
	_, err := docker.ParseInfoJSON("id", b)
	c.Assert(err, gc.ErrorMatches, "can't decode response from docker inspect id.*")
}

func (infoSuite) TestParseInfoJSONEmpty(c *gc.C) {
	b := []byte(`[]`)
	_, err := docker.ParseInfoJSON("id", b)
	c.Assert(err, gc.ErrorMatches, "no status returned from docker inspect id")
}

func (infoSuite) TestParseInfoJSONMultiple(c *gc.C) {
	b := []byte(`[{"Name":"foo"},{"Name":"bar"}]`)
	_, err := docker.ParseInfoJSON("id", b)
	c.Assert(err, gc.ErrorMatches, "multiple status values returned from docker inspect id")
}

func (infoSuite) TestStateValue(c *gc.C) {
	info := docker.Info(types.ContainerJSONPre120{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232",
			Name: "/sad_perlman",
			State: &types.ContainerState{
				Running:    true,
				Paused:     false,
				Restarting: false,
				OOMKilled:  false,
				Dead:       false,
				Pid:        11820,
			},
		},
	})
	state := info.StateValue()

	c.Check(state, gc.Equals, docker.StateRunning)
}

const fakeInspectOutput = `
[
{
    "Id": "b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232",
    "Created": "2015-06-25T11:05:53.694518797Z",
    "Path": "sleep",
    "Args": [
        "30"
    ],
    "State": {
        "Running": true,
        "Paused": false,
        "Restarting": false,
        "OOMKilled": false,
        "Dead": false,
        "Pid": 11820,
        "ExitCode": 0,
        "Error": "",
        "StartedAt": "2015-06-25T11:05:53.8401024Z",
        "FinishedAt": "0001-01-01T00:00:00Z"
    },
    "Image": "fb434121fc77c965f255cbb848927f577bbdbd9325bdc1d7f1b33f99936b9abb",
    "NetworkSettings": {
        "Bridge": "",
        "EndpointID": "9915c7299be4f77c18f3999ef422b79996ea8c5796e2befd1442d67e5cefb50d",
        "Gateway": "172.17.42.1",
        "GlobalIPv6Address": "",
        "GlobalIPv6PrefixLen": 0,
        "HairpinMode": false,
        "IPAddress": "172.17.0.2",
        "IPPrefixLen": 16,
        "IPv6Gateway": "",
        "LinkLocalIPv6Address": "",
        "LinkLocalIPv6PrefixLen": 0,
        "MacAddress": "02:42:ac:11:00:02",
        "NetworkID": "3346546be8f76006e44000b007da48e576e788ba1d3e3cd275545837d4d7c80a",
        "PortMapping": null,
        "Ports": {},
        "SandboxKey": "/var/run/docker/netns/b508c7d5c272",
        "SecondaryIPAddresses": null,
        "SecondaryIPv6Addresses": null
    },
    "ResolvConfPath": "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/resolv.conf",
    "HostnamePath": "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/hostname",
    "HostsPath": "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/hosts",
    "LogPath": "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232-json.log",
    "Name": "/sad_perlman",
    "RestartCount": 0,
    "Driver": "aufs",
    "ExecDriver": "native-0.2",
    "MountLabel": "",
    "ProcessLabel": "",
    "Volumes": {},
    "VolumesRW": {},
    "AppArmorProfile": "",
    "ExecIDs": null,
    "HostConfig": {
        "Binds": null,
        "ContainerIDFile": "",
        "LxcConf": [],
        "Memory": 0,
        "MemorySwap": 0,
        "CpuShares": 0,
        "CpuPeriod": 0,
        "CpusetCpus": "",
        "CpusetMems": "",
        "CpuQuota": 0,
        "BlkioWeight": 0,
        "OomKillDisable": false,
        "Privileged": false,
        "PortBindings": {},
        "Links": null,
        "PublishAllPorts": false,
        "Dns": null,
        "DnsSearch": null,
        "ExtraHosts": null,
        "VolumesFrom": null,
        "Devices": [],
        "NetworkMode": "bridge",
        "IpcMode": "",
        "PidMode": "",
        "UTSMode": "",
        "CapAdd": null,
        "CapDrop": null,
        "RestartPolicy": {
            "Name": "no",
            "MaximumRetryCount": 0
        },
        "SecurityOpt": null,
        "ReadonlyRootfs": false,
        "Ulimits": null,
        "LogConfig": {
            "Type": "json-file",
            "Config": {}
        },
        "CgroupParent": ""
    },
    "Config": {
        "Hostname": "b508c7d5c272",
        "Domainname": "",
        "User": "",
        "AttachStdin": false,
        "AttachStdout": false,
        "AttachStderr": false,
        "PortSpecs": null,
        "ExposedPorts": null,
        "Tty": false,
        "OpenStdin": false,
        "StdinOnce": false,
        "Env": [
            "PATH=/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
        ],
        "Cmd": [
            "sleep",
            "30"
        ],
        "Image": "docker/whalesay",
        "Volumes": null,
        "VolumeDriver": "",
        "WorkingDir": "/cowsay",
        "Entrypoint": null,
        "NetworkDisabled": false,
        "MacAddress": "",
        "OnBuild": null,
        "Labels": {}
    }
}
]
`

var fakeInfo = &types.ContainerJSONPre120{
	ContainerJSONBase: &types.ContainerJSONBase{
		ID:      "b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232",
		Created: "2015-06-25T11:05:53.694518797Z",
		Path:    "sleep",
		Args: []string{
			"30",
		},
		State: &types.ContainerState{
			Running:    true,
			Paused:     false,
			Restarting: false,
			OOMKilled:  false,
			Dead:       false,
			Pid:        11820,
			ExitCode:   0,
			Error:      "",
			StartedAt:  "2015-06-25T11:05:53.8401024Z",
			FinishedAt: "0001-01-01T00:00:00Z",
		},
		Image: "fb434121fc77c965f255cbb848927f577bbdbd9325bdc1d7f1b33f99936b9abb",
		NetworkSettings: &network.Settings{
			Bridge:                 "",
			EndpointID:             "9915c7299be4f77c18f3999ef422b79996ea8c5796e2befd1442d67e5cefb50d",
			Gateway:                "172.17.42.1",
			GlobalIPv6Address:      "",
			GlobalIPv6PrefixLen:    0,
			HairpinMode:            false,
			IPAddress:              "172.17.0.2",
			IPPrefixLen:            16,
			IPv6Gateway:            "",
			LinkLocalIPv6Address:   "",
			LinkLocalIPv6PrefixLen: 0,
			MacAddress:             "02:42:ac:11:00:02",
			NetworkID:              "3346546be8f76006e44000b007da48e576e788ba1d3e3cd275545837d4d7c80a",
			PortMapping:            nil,
			Ports:                  nat.PortMap{},
			SandboxKey:             "/var/run/docker/netns/b508c7d5c272",
			SecondaryIPAddresses:   nil,
			SecondaryIPv6Addresses: nil,
		},
		ResolvConfPath:  "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/resolv.conf",
		HostnamePath:    "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/hostname",
		HostsPath:       "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/hosts",
		LogPath:         "/var/lib/docker/containers/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232/b508c7d5c2722b7ac4f105fedf835789fb705f71feb6e264f542dc33cdc41232-json.log",
		Name:            "/sad_perlman",
		RestartCount:    0,
		Driver:          "aufs",
		ExecDriver:      "native-0.2",
		MountLabel:      "",
		ProcessLabel:    "",
		AppArmorProfile: "",
		ExecIDs:         nil,
		HostConfig: &runconfig.HostConfig{
			Binds:           nil,
			ContainerIDFile: "",
			LxcConf:         &runconfig.LxcConfig{},
			Memory:          0,
			MemorySwap:      0,
			CPUShares:       0,
			CPUPeriod:       0,
			CpusetCpus:      "",
			CpusetMems:      "",
			CPUQuota:        0,
			BlkioWeight:     0,
			OomKillDisable:  false,
			Privileged:      false,
			PortBindings:    nat.PortMap{},
			Links:           nil,
			PublishAllPorts: false,
			DNS:             nil,
			DNSSearch:       nil,
			ExtraHosts:      nil,
			VolumesFrom:     nil,
			Devices:         nil,
			NetworkMode:     "bridge",
			IpcMode:         "",
			PidMode:         "",
			UTSMode:         "",
			CapAdd:          nil,
			CapDrop:         nil,
			RestartPolicy: runconfig.RestartPolicy{
				Name:              "no",
				MaximumRetryCount: 0,
			},
			SecurityOpt:    nil,
			ReadonlyRootfs: false,
			Ulimits:        nil,
			LogConfig: runconfig.LogConfig{
				Type:   "json-file",
				Config: map[string]string{},
			},
			CgroupParent: "",
		},
	},
	Volumes:   map[string]string{},
	VolumesRW: map[string]bool{},
	Config: &types.ContainerConfig{
		Config: &runconfig.Config{
			Hostname:     "b508c7d5c272",
			Domainname:   "",
			User:         "",
			AttachStdin:  false,
			AttachStdout: false,
			AttachStderr: false,
			//PortSpecs:    nil,
			ExposedPorts: nil,
			Tty:          false,
			OpenStdin:    false,
			StdinOnce:    false,
			Env: []string{
				"PATH=/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			Cmd: runconfig.NewCommand(
				"sleep",
				"30",
			),
			Image:           "docker/whalesay",
			Volumes:         nil,
			VolumeDriver:    "",
			WorkingDir:      "/cowsay",
			Entrypoint:      nil,
			NetworkDisabled: false,
			MacAddress:      "",
			OnBuild:         nil,
			Labels:          map[string]string{},
		},
	},
}
