#!/usr/bin/python3
import subprocess
import re

from charmhelpers.core.hookenv import (
    action_set
)


def main():
    unit_interfaces = interfaces()
    action_set({'interfaces': unit_interfaces})


def interfaces():
    raw = subprocess.check_output('ifconfig', shell=True).decode("utf-8")
    patterns = ['(?P<device>^[a-zA-Z0-9:]+)(.*)Link encap:(.*).*',
                '(.*)Link encap:(.*)(HWaddr )(?P<ether>[^\s]*).*',
                '.*(inet addr:)(?P<inet>[^\s]*).*',
                '.*(inet6 addr: )(?P<inet6>[^\s\/]*/(?P<prefixlen>[\d]*)).*',
                '.*(P-t-P:)(?P<ptp>[^\s]*).*',
                '.*(Bcast:)(?P<broadcast>[^\s]*).*',
                '.*(Mask:)(?P<netmask>[^\s]*).*',
                '.*(Scope:)(?P<scopeid>[^\s]*).*',
                '.*(RX bytes:)(?P<rxbytes>\d+).*',
                '.*(TX bytes:)(?P<txbytes>\d+).*']
    interfaces = {}
    cur = None
    all_keys = []

    for line in raw.splitlines():
        for pattern in patterns:
            match = re.search(pattern, line)
            if match:
                groupdict = match.groupdict()
            if 'device' in groupdict:
                cur = groupdict['device']
                if cur not in interfaces:
                    interfaces[cur] = {}

            for key in groupdict:
                if key not in all_keys:
                    all_keys.append(key)
                interfaces[cur][key] = groupdict[key]
    return interfaces


if __name__ == "__main__":
    main()
