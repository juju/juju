#!/usr/bin/python3
import subprocess
import re

from charmhelpers.core.hookenv import (
    action_set
)


def main():
    unit_interfaces = interfaces()
    dns_info = dns(unit_interfaces)
    action_set({'dns': dns_info, 'interfaces': unit_interfaces})


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


def dns(interfaces):
    link_dns = {}
    for interface, info in interfaces.items():
        link_dns[interface] = {'ipv4': None, 'ipv6': None}
        if info.get('inet'):
            link_dns[interface]['ipv4'] = get_dns(interface)
        if info.get('inet6'):
            link_dns[interface]['ipv6'] = get_dns(interface, ipv6=True)
    return link_dns


def get_dns(interface, ipv6=False):
    raw = None
    if ipv6:
        ver = '6'
    else:
        ver = '4'
    cmd = 'nmcli device show {} | grep IP{}.DNS'.format(interface, ver)
    try:
        raw = subprocess.check_output(cmd, shell=True).decode("utf-8")
    except subprocess.CalledProcessError as e:
        print('Could not get dns due to error:\n {}'.format(e))
    if raw:
        return raw.split()[1]
    return None


if __name__ == "__main__":
    main()
