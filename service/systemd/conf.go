// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/coreos/go-systemd/unit"
	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
)

var limitMap = map[string]string{
	"as":         "LimitAS",
	"core":       "LimitCORE",
	"cpu":        "LimitCPU",
	"data":       "LimitDATA",
	"fsize":      "LimitFSIZE",
	"memlock":    "LimitMEMLOCK",
	"msgqueue":   "LimitMSGQUEUE",
	"nice":       "LimitNICE",
	"nofile":     "LimitNOFILE",
	"nproc":      "LimitNPROC",
	"rss":        "LimitRSS",
	"rtprio":     "LimitRTPRIO",
	"sigpending": "LimitSIGPENDING",
	"stack":      "LimitSTACK",
}

// normalize adjusts the conf to more standardized content and
// returns a new Conf with that updated content. It also returns the
// content of any script file that should accompany the conf.
func normalize(conf common.Conf, scriptPath string) (common.Conf, []byte) {
	var data []byte

	if conf.ExtraScript != "" {
		conf.ExecStart = conf.ExtraScript + "\n" + conf.ExecStart
		conf.ExtraScript = ""
	}
	if strings.Contains(conf.ExecStart, "\n") {
		data = []byte(conf.ExecStart)
		conf.ExecStart = scriptPath
	}

	if len(conf.Env) == 0 {
		conf.Env = nil
	}

	if len(conf.Limit) == 0 {
		conf.Limit = nil
	}

	return conf, data
}

func validate(name string, conf common.Conf) error {
	if name == "" {
		return errors.NotValidf("missing service name")
	}

	if conf.ExecStart == "" {
		return errors.NotValidf("missing ExecStart")
	} else if !strings.HasPrefix(conf.ExecStart, "/") {
		return errors.NotValidf("relative path in ExecStart")
	}

	if conf.ExtraScript != "" {
		return errors.NotValidf("unexpected ExtraScript")
	}

	if conf.Output != "" && conf.Output != "syslog" {
		return errors.NotValidf("conf.Output value %q (Options are syslog)", conf.Output)
	}
	// We ignore Desc and InitDir.

	for k := range conf.Limit {
		if _, ok := limitMap[k]; !ok {
			return errors.NotValidf("conf.Limit key %q", k)
		}
	}

	if conf.Transient {
		// TODO(ericsnow) This needs to be sorted out.
		return errors.NotSupportedf("Conf.Transient")
	}

	if conf.AfterStopped != "" {
		// TODO(ericsnow) This needs to be sorted out.
		return errors.NotSupportedf("Conf.AfterStopped")
	}

	if conf.ExecStopPost != "" {
		// TODO(ericsnow) This needs to be sorted out.
		return errors.NotSupportedf("Conf.ExecStopPost")

		if !strings.HasPrefix(conf.ExecStopPost, "/") {
			return errors.NotValidf("relative path in ExecStopPost")
		}
	}

	return nil
}

// serialize returns the data that should be written to disk for the
// provided Conf, rendered in the systemd unit file format.
func serialize(name string, conf common.Conf) ([]byte, error) {
	if err := validate(name, conf); err != nil {
		return nil, errors.Trace(err)
	}

	var unitOptions []*unit.UnitOption
	unitOptions = append(unitOptions, serializeUnit(conf)...)
	unitOptions = append(unitOptions, serializeService(conf)...)
	unitOptions = append(unitOptions, serializeInstall(conf)...)

	data, err := ioutil.ReadAll(unit.Serialize(unitOptions))
	return data, errors.Trace(err)
}

func serializeUnit(conf common.Conf) []*unit.UnitOption {
	var unitOptions []*unit.UnitOption

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Unit",
		Name:    "Description",
		Value:   conf.Desc,
	})

	after := []string{
		"syslog.target",
		"network.target",
		"systemd-user-sessions.service",
	}
	for _, name := range after {
		unitOptions = append(unitOptions, &unit.UnitOption{
			Section: "Unit",
			Name:    "After",
			Value:   name,
		})
	}

	return unitOptions
}

func serializeService(conf common.Conf) []*unit.UnitOption {
	var unitOptions []*unit.UnitOption

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Service",
		Name:    "Type",
		Value:   "forking",
	})

	if conf.Output != "" {
		unitOptions = append(unitOptions, &unit.UnitOption{
			Section: "Service",
			Name:    "StandardOutput",
			Value:   conf.Output,
		})
		unitOptions = append(unitOptions, &unit.UnitOption{
			Section: "Service",
			Name:    "StandardError",
			Value:   conf.Output,
		})
	}

	for k, v := range conf.Env {
		unitOptions = append(unitOptions, &unit.UnitOption{
			Section: "Service",
			Name:    "Environment",
			Value:   fmt.Sprintf(`"%q=%q"`, k, v),
		})
	}

	for k, v := range conf.Limit {
		unitOptions = append(unitOptions, &unit.UnitOption{
			Section: "Service",
			Name:    limitMap[k],
			Value:   v,
		})
	}

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Service",
		Name:    "ExecStart",
		Value:   conf.ExecStart,
	})

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Service",
		Name:    "RemainAfterExit",
		Value:   "yes",
	})

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Service",
		Name:    "Restart",
		Value:   "always",
	})

	return unitOptions
}

func serializeInstall(conf common.Conf) []*unit.UnitOption {
	var unitOptions []*unit.UnitOption

	unitOptions = append(unitOptions, &unit.UnitOption{
		Section: "Install",
		Name:    "WantedBy",
		Value:   "multi-user.target",
	})

	return unitOptions
}

// deserialize parses the provided data (in the systemd unit file
// format) and populates a new Conf with the result.
func deserialize(data []byte) (common.Conf, error) {
	opts, err := unit.Deserialize(bytes.NewBuffer(data))
	if err != nil {
		return common.Conf{}, errors.Trace(err)
	}
	return deserializeOptions(opts)
}

func deserializeOptions(opts []*unit.UnitOption) (common.Conf, error) {
	var conf common.Conf

	for _, uo := range opts {
		switch uo.Section {
		case "Unit":
			switch uo.Name {
			case "Description":
				conf.Desc = uo.Value
			case "After":
				// Do nothing until we support it in common.Conf.
			default:
				return conf, errors.NotSupportedf("Unit directive %q", uo.Name)
			}
		case "Service":
			switch {
			case uo.Name == "ExecStart":
				conf.ExecStart = uo.Value
			case uo.Name == "StandardError", uo.Name == "StandardOutput":
				// TODO(wwitzel3) We serialize Standard(Error|Output)
				// to the same thing, but we should probably make sure they match
				conf.Output = uo.Value
			case uo.Name == "Environment":
				if conf.Env == nil {
					conf.Env = make(map[string]string)
				}
				var value = uo.Value
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					value = value[1 : len(value)-1]
				}
				parts := strings.SplitN(value, "=", 2)
				if len(parts) != 2 {
					return conf, errors.NotValidf("service environment value %q", uo.Value)
				}
				conf.Env[parts[0]] = parts[1]
			case strings.HasPrefix(uo.Name, "Limit"):
				if conf.Limit == nil {
					conf.Limit = make(map[string]string)
				}
				for k, v := range limitMap {
					if v == uo.Name {
						conf.Limit[k] = v
						break
					}
				}
			case uo.Name == "Type":
				// Do nothing until we support it in common.Conf.
			case uo.Name == "RemainAfterExit":
				// Do nothing until we support it in common.Conf.
			case uo.Name == "Restart":
				// Do nothing until we support it in common.Conf.
			default:
				return conf, errors.NotSupportedf("Service directive %q", uo.Name)
			}

		case "Install":
			switch uo.Name {
			case "WantedBy":
				if uo.Value != "multi-user.target" {
					return conf, errors.NotValidf("unit target %q", uo.Value)
				}
			default:
				return conf, errors.NotSupportedf("Install directive %q", uo.Name)
			}
		default:
			return conf, errors.NotSupportedf("section %q", uo.Name)
		}
	}

	err := validate("<>", conf)
	return conf, errors.Trace(err)
}
