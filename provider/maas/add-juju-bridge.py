#!/usr/bin/env python

# Copyright 2015 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

#
# This file has been and should be formatted using pyfmt(1).
#

from __future__ import print_function
import argparse
import os
import re
import shutil
import subprocess
import sys


class SeekableIterator(object):
    """An iterator that supports relative seeking."""

    def __init__(self, iterable):
        self.iterable = iterable
        self.index = 0

    def __iter__(self):
        return self

    def next(self):  # Python 2
        try:
            value = self.iterable[self.index]
            self.index += 1
            return value
        except IndexError:
            raise StopIteration

    def __next__(self):  # Python 3
        return self.next()

    def seek(self, n, relative=False):
        if relative:
            self.index += n
        else:
            self.index = n
        if self.index < 0 or self.index >= len(self.iterable):
            raise IndexError


class PhysicalInterface(object):
    """Represents a physical ('auto') interface."""

    def __init__(self, definition):
        self.name = definition.split()[1]

    def __str__(self):
        return self.name


class LogicalInterface(object):
    """Represents a logical ('iface') interface."""

    def __init__(self, definition, options=None):
        if not options:
            options = []
        _, self.name, self.family, self.method = definition.split()
        self.options = options
        self.is_bonded = [x for x in self.options if "bond-" in x]
        self.is_alias = ":" in self.name
        self.is_vlan = [x for x in self.options if x.startswith("vlan-raw-device")]
        self.is_active = self.method == "dhcp" or self.method == "static"

    def __str__(self):
        return self.name

    # Returns an ordered set of stanzas to bridge this interface.
    def bridge(self, prefix, bridge_name, add_auto_stanza):
        if bridge_name is None:
            bridge_name = prefix + self.name
        # Note: the testing order here is significant.
        if not self.is_active:
            return self._bridge_inactive(add_auto_stanza)
        elif self.is_alias:
            return self._bridge_alias(add_auto_stanza)
        elif self.is_vlan:
            return self._bridge_vlan(prefix, bridge_name, add_auto_stanza)
        elif self.is_bonded:
            return self._bridge_bond(prefix, bridge_name, add_auto_stanza)
        else:
            return self._bridge_device(prefix, bridge_name)

    def _bridge_device(self, prefix, bridge_name):
        s1 = IfaceStanza(self.name, self.family, "manual", [])
        s2 = AutoStanza(bridge_name)
        options = list(self.options)
        options.append("bridge_ports {}".format(self.name))
        s3 = IfaceStanza(bridge_name, self.family, self.method, options)
        return [s1, s2, s3]

    def _bridge_vlan(self, prefix, bridge_name, add_auto_stanza):
        stanzas = []
        s1 = IfaceStanza(self.name, self.family, "manual", self.options)
        stanzas.append(s1)
        if add_auto_stanza:
            stanzas.append(AutoStanza(bridge_name))
        options = [x for x in self.options if not x.startswith("vlan")]
        options.append("bridge_ports {}".format(self.name))
        s3 = IfaceStanza(bridge_name, self.family, self.method, options)
        stanzas.append(s3)
        return stanzas

    def _bridge_alias(self, add_auto_stanza):
        stanzas = []
        if add_auto_stanza:
            stanzas.append(AutoStanza(self.name))
        s1 = IfaceStanza(self.name, self.family, self.method, list(self.options))
        stanzas.append(s1)
        return stanzas

    def _bridge_bond(self, prefix, bridge_name, add_auto_stanza):
        stanzas = []
        if add_auto_stanza:
            stanzas.append(AutoStanza(self.name))
        s1 = IfaceStanza(self.name, self.family, "manual", list(self.options))
        s2 = AutoStanza(bridge_name)
        options = [x for x in self.options if not x.startswith("bond")]
        options.append("bridge_ports {}".format(self.name))
        s3 = IfaceStanza(bridge_name, self.family, self.method, options)
        stanzas.extend([s1, s2, s3])
        return stanzas

    def _bridge_inactive(self, add_auto_stanza):
        stanzas = []
        if add_auto_stanza:
            stanzas.append(AutoStanza(self.name))
        s1 = IfaceStanza(self.name, self.family, self.method, list(self.options))
        stanzas.append(s1)
        return stanzas


class Stanza(object):
    """Represents one stanza together with all of its options."""

    def __init__(self, definition, options=None):
        if not options:
            options = []
        self.definition = definition
        self.options = options
        self.is_logical_interface = definition.startswith('iface ')
        self.is_physical_interface = definition.startswith('auto ')
        self.iface = None
        self.phy = None
        if self.is_logical_interface:
            self.iface = LogicalInterface(definition, self.options)
        if self.is_physical_interface:
            self.phy = PhysicalInterface(definition)

    def __str__(self):
        return self.definition


class NetworkInterfaceParser(object):
    """Parse a network interface file into a set of stanzas."""

    @classmethod
    def is_stanza(cls, s):
        return re.match(r'^(iface|mapping|auto|allow-|source)', s)

    def __init__(self, filename):
        self._stanzas = []
        with open(filename, 'r') as f:
            lines = f.readlines()
        line_iterator = SeekableIterator(lines)
        for line in line_iterator:
            if self.is_stanza(line):
                stanza = self._parse_stanza(line, line_iterator)
                self._stanzas.append(stanza)

    def _parse_stanza(self, stanza_line, iterable):
        stanza_options = []
        for line in iterable:
            line = line.strip()
            if line.startswith('#') or line == "":
                continue
            if self.is_stanza(line):
                iterable.seek(-1, True)
                break
            stanza_options.append(line)
        return Stanza(stanza_line.strip(), stanza_options)

    def stanzas(self):
        return [x for x in self._stanzas]

    def physical_interfaces(self):
        return {x.phy.name: x.phy for x in [y for y in self._stanzas if y.is_physical_interface]}

    def __iter__(self):  # class iter
        for s in self._stanzas:
            yield s


def uniq_append(dst, src):
    for x in src:
        if x not in dst:
            dst.append(x)
    return dst


def IfaceStanza(name, family, method, options):
    """Convenience function to create a new "iface" stanza.

Maintains original options order but removes duplicates with the
exception of 'dns-*' options which are normlised as required by
resolvconf(8) and all the dns-* options are moved to the end.

    """

    dns_search = []
    dns_nameserver = []
    dns_sortlist = []
    unique_options = []

    for o in options:
        words = o.split()
        ident = words[0]
        if ident == "dns-nameservers":
            dns_nameserver = uniq_append(dns_nameserver, words[1:])
        elif ident == "dns-search":
            dns_search = uniq_append(dns_search, words[1:])
        elif ident == "dns-sortlist":
            dns_sortlist = uniq_append(dns_sortlist, words[1:])
        elif o not in unique_options:
            unique_options.append(o)

    if dns_nameserver:
        option = "dns-nameservers " + " ".join(dns_nameserver)
        unique_options.append(option)

    if dns_search:
        option = "dns-search " + " ".join(dns_search)
        unique_options.append(option)

    if dns_sortlist:
        option = "dns-sortlist " + " ".join(dns_sortlist)
        unique_options.append(option)

    return Stanza("iface {} {} {}".format(name, family, method), unique_options)


def AutoStanza(name):
    # Convenience function to create a new "auto" stanza.
    return Stanza("auto {}".format(name))


def print_stanza(s, stream=sys.stdout):
    print(s.definition, file=stream)
    for o in s.options:
        print("   ", o, file=stream)


def print_stanzas(stanzas, stream=sys.stdout):
    n = len(stanzas)
    for i, stanza in enumerate(stanzas):
        print_stanza(stanza, stream)
        if stanza.is_logical_interface and i + 1 < n:
            print(file=stream)


def shell_cmd(s):
    p = subprocess.Popen(s, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, err = p.communicate()
    return [out, err, p.returncode]


def print_shell_cmd(s, verbose=True, exit_on_error=False):
    if verbose:
        print(s)
    out, err, retcode = shell_cmd(s)
    if out and len(out) > 0:
        print(out.decode().rstrip('\n'))
    if err and len(err) > 0:
        print(err.decode().rstrip('\n'))
    if exit_on_error and retcode != 0:
        exit(1)


def check_shell_cmd(s, verbose=False):
    if verbose:
        print(s)
    output = subprocess.check_output(s, shell=True, stderr=subprocess.STDOUT).strip().decode("utf-8")
    if verbose:
        print(output.rstrip('\n'))
    return output


def arg_parser():
    parser = argparse.ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument('--bridge-prefix', help="bridge prefix", type=str, required=False, default='br-')
    parser.add_argument('--one-time-backup', help='A one time backup of filename', action='store_true', default=True, required=False)
    parser.add_argument('--activate', help='activate new configuration', action='store_true', default=False, required=False)
    parser.add_argument('--interface-to-bridge', help="interface to bridge", type=str, required=False)
    parser.add_argument('--bridge-name', help="bridge name", type=str, required=False)
    parser.add_argument('filename', help="interfaces(5) based filename")
    return parser


def main(args):
    if args.bridge_name and args.interface_to_bridge is None:
        sys.stderr.write("error: --interface-to-bridge required when using --bridge-name\n")
        exit(1)

    if args.interface_to_bridge and args.bridge_name is None:
        sys.stderr.write("error: --bridge-name required when using --interface-to-bridge\n")
        exit(1)

    stanzas = []
    config_parser = NetworkInterfaceParser(args.filename)
    physical_interfaces = config_parser.physical_interfaces()

    # Bridging requires modifying 'auto' and 'iface' stanzas only.
    # Calling <iface>.bridge() will return a set of stanzas that cover
    # both of those stanzas. The 'elif' clause catches all the other
    # stanza types. The args.interface_to_bridge test is to bridge a
    # single interface only, which is only used for juju < 2.0. And if
    # that argument is specified then args.bridge_name takes
    # precendence over any args.bridge_prefix.

    for s in config_parser.stanzas():
        if s.is_logical_interface:
            add_auto_stanza = s.iface.name in physical_interfaces
            if args.interface_to_bridge and args.interface_to_bridge != s.iface.name:
                if add_auto_stanza:
                    stanzas.append(AutoStanza(s.iface.name))
                stanzas.append(s)
            else:
                stanzas.extend(s.iface.bridge(args.bridge_prefix, args.bridge_name, add_auto_stanza))
        elif not s.is_physical_interface:
            stanzas.append(s)

    if not args.activate:
        print_stanzas(stanzas)
        exit(0)

    if args.one_time_backup:
        backup_file = "{}-before-add-juju-bridge".format(args.filename)
        if not os.path.isfile(backup_file):
            shutil.copy2(args.filename, backup_file)

    ifquery = "$(ifquery --interfaces={} --exclude=lo --list)".format(args.filename)

    print("**** Original configuration")
    print_shell_cmd("cat {}".format(args.filename))
    print_shell_cmd("ifconfig -a")
    print_shell_cmd("ifdown --exclude=lo --interfaces={} {}".format(args.filename, ifquery))

    print("**** Activating new configuration")

    with open(args.filename, 'w') as f:
        print_stanzas(stanzas, f)
        f.close()

    print_shell_cmd("cat {}".format(args.filename))
    print_shell_cmd("ifup --exclude=lo --interfaces={} {}".format(args.filename, ifquery))
    print_shell_cmd("ip link show up")
    print_shell_cmd("ifconfig -a")
    print_shell_cmd("ip route show")
    print_shell_cmd("brctl show")

# This script re-renders an interfaces(5) file to add a bridge to
# either all active interfaces, or a specific interface.

if __name__ == '__main__':
    main(arg_parser().parse_args())
