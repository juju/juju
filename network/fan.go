package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
)

type FanConfigEntry struct {
	Underlay *net.IPNet
	Overlay  *net.IPNet
}

type FanConfig []FanConfigEntry

func ParseFanConfig(line string) (config FanConfig, err error) {
	if line == "" {
		return nil, nil
	}
	entries := strings.Split(line, ";")
	config = make([]FanConfigEntry, len(entries))
	for i, line := range entries {
		cidrs := strings.Split(line, ":")
		if len(cidrs) != 2 {
			return nil, fmt.Errorf("invalid FAN config entry: %v", line)
		}
		if _, config[i].Underlay, err = net.ParseCIDR(strings.TrimSpace(cidrs[0])); err != nil {
			return nil, errors.Annotatef(err, "invalid address in FAN config")
		}
		if _, config[i].Overlay, err = net.ParseCIDR(strings.TrimSpace(cidrs[1])); err != nil {
			return nil, errors.Annotatef(err, "invalid address in FAN config")
		}
		underlaySize, _ := config[i].Underlay.Mask.Size()
		overlaySize, _ := config[i].Overlay.Mask.Size()
		if underlaySize <= overlaySize {
			return nil, fmt.Errorf("invalid FAN config, underlay mask must be larger than overlay: %s", line)
		}
	}
	return config, nil
}

func (fc *FanConfig) String() (line string) {
	configs := make([]string, len(*fc))
	for i, fan := range *fc {
		configs[i] = fmt.Sprintf("%s:%s", fan.Underlay.String(), fan.Overlay.String())
	}
	return strings.Join(configs, ";")
}
