#!/usr/bin/env python
"""Assess Juju under various proxy network conditions.

This test is dangerous to run on your own host. It will change the host's
network and can lock you out of your host. There are checks to ensure the
host matches expectations so that the network can be reset. While the test
is running, other processes on the host may be crippled.
"""

from __future__ import print_function

import argparse
import logging
import os
import re
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_proxy")

ENVIRONMENT_FILE = '/etc/environment'

SCENARIO_BOTH = 'both-proxied'
SCENARIO_CLIENT = 'client-proxied'
SCENARIO_CONTROLLER = 'controller-proxied'

IPTABLES_FORWARD_PROXY = '-A FORWARD -i {} -p tcp --d port 3128 -j ACCEPT'
IPTABLES_BACKUP = '/etc/iptables.before-assess-proxy'

UFW_RESET_COMMANDS = [
    ('sudo', 'iptables-restore', IPTABLES_BACKUP),
    ('sudo', 'ufw', '--force', 'reset'),
    ('sudo', 'ufw', '--force', 'disable'),
]


class UndefinedProxyError(Exception):
    """The current env or /etc/environment does not define proxy info."""


def get_environment_file_path():
    return ENVIRONMENT_FILE


def check_environment():
    """Verify the the current env and etc/environment' define a proxy.

    Check for lowercase names because Juju uses them, some charms/apps might
    want uppercase names. This check assumes the env has both upper and lower
    case forms.
    """
    http_proxy = os.environ.get('http_proxy', None)
    https_proxy = os.environ.get('https_proxy', None)
    if http_proxy is None or https_proxy is None:
        message = 'http_proxy and https_proxy not defined in env'
        log.error(message)
        raise UndefinedProxyError(message)
    try:
        with open(get_environment_file_path(), 'r') as env_file:
            env_data = env_file.read()
    except IOError:
        env_data = ''
    found_http_proxy = False
    found_https_proxy = False
    for line in env_data.splitlines():
        if '=' in line:
            key, value = line.split('=', 1)
            if key == 'http_proxy' and value == http_proxy:
                found_http_proxy = True
            elif key == 'https_proxy' and value == https_proxy:
                found_https_proxy = True
    if not found_http_proxy or not found_https_proxy:
        message = (
            'http_proxy and https_proxy not defined in /etc/environment')
        log.error(message)
        raise UndefinedProxyError(message)
    return http_proxy, https_proxy


def check_network(client_interface, controller_interface):
    """Verify the interfaces are usable and return the FORWARD IN rule.

    :raises ValueError: when the interfaces are not present or the FORWARD IN
        rule cannot be identified.
    :return: the FORWARD IN rule that must be restored before the test exits.
    """
    if subprocess.call(['ifconfig', client_interface]) != 0:
        message = 'client_interface {} not found'.format(client_interface)
        log.error(message)
        raise ValueError(message)
    if subprocess.call(['ifconfig', controller_interface]) != 0:
        message = 'controller_interface {} not found'.format(
            controller_interface)
        log.error(message)
        raise ValueError(message)
    # We need to match a single rule from iptables:
    # sudo iptables -S lxdbr0
    # -A FORWARD -i lxdbr0 -m comment --comment "managed by lxd" -j ACCEPT
    rules = subprocess.check_output(['sudo', 'iptables', '-S', 'FORWARD'])
    forward_pattern = re.compile(
        '(-A FORWARD -i {}.*-j ACCEPT)'.format(controller_interface))
    forward_rule = None
    for rule in rules.splitlines():
        match = forward_pattern.search(rule)
        if match and forward_rule is None:
            forward_rule = match.group(1)
        elif match and forward_rule is not None:
            # There is more than one match. We did not match a unique to
            # delete and restore.
            forward_rule = None
            break
    if forward_rule is None:
        # Either the rule was not matched or it was matched more than once.
        raise ValueError(
            'Cannot identify the unique iptables FOWARD IN rule for {}'.format(
                controller_interface))
    return forward_rule


def set_firewall(scenario, forward_rule):
    """Setup the firewall to match the scenario."""
    pass


def reset_firewall():
    """Reset the firewall and disable it.

    The firewall's rules are reset, then it is disabled. The ufw reset command
    implicitly disables, but disable is explicitly called to ensure ufw
    is not running. iptables-restore it called with the .before-assess-proxy
    backup.
    """
    errors = []
    for command in UFW_RESET_COMMANDS:
        try:
            subprocess.check_call(command)
            log.info('{} exited successfully'.format(command))
        except subprocess.CalledProcessError as e:
            errors.append(e)
            log.error('{} exited with {}'.format(e.cmd, e.returncode))
            log.error('This host may be in a dirty state.')
    return errors


def assess_proxy(client, scenario):
    client.deploy('cs:xenial/ubuntu')
    client.wait_for_started()
    client.wait_for_workloads()
    log.info("SUCCESS")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Assess Juju under various proxy network conditions.")
    add_basic_testing_arguments(parser)
    parser.add_argument(
        'scenario', choices=[
            SCENARIO_BOTH, SCENARIO_CLIENT, SCENARIO_CONTROLLER],
        help="The proxy scenario to run.")
    parser.add_argument(
        '--client-interface', default='eth0',
        help="The interface used by the client to access the internet.")
    parser.add_argument(
        '--controller-interface', default='lxdbr0',
        help="The interface used by the controller to access the internet.")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    log.info("Checking the setup of the network and firewall")
    forward_rule = check_network(
        args.client_interface, args.controller_interface)
    try:
        log.info("Setting firewall")
        set_firewall(args.scenario, forward_rule)
        log.info("Starting test")
        bs_manager = BootstrapManager.from_args(args)
        log.info("Starting bootstrap")
        with bs_manager.booted_context(args.upload_tools):
            log.info("PASS bootstrap")
            assess_proxy(bs_manager.client, args.scenario)
            log.info("Finished test")
    finally:
        # Always reopen the network, regardless of what happened.
        # Do not lockout the host.
        log.info("Resetting firewall")
        reset_firewall()
    return 0


if __name__ == '__main__':
    sys.exit(main())
