#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import os
import subprocess
import sys
from time import sleep

from jujupy import (
    CannotConnectEnv,
    Environment,
    start_libvirt_domain,
    stop_libvirt_domain,
    verify_libvirt_domain_running,
)


def deploy_stack(environment, debug, machines):
    """"Deploy a test stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    env = Environment.from_config(environment)
    env.client.debug = debug
    running_domains = dict()
    if env.config['type'] == 'maas':
        # Split the hypervisor_URI and machine name
        for machine in machines:
            name, URI = machine.split('@')
            # Record already running domains, so they can be left running,
            # if already running; otherwise start them.
            if verify_libvirt_domain_running(URI, name):
                running_domains = {machine: True}
            else:
                running_domains = {machine: False}
                print("Attempting to start %s at %s" % (name, URI))
                status_msg = start_libvirt_domain(URI, name)
                print("%s" % status_msg)
    # Clean up any leftover junk
    env.destroy_environment()
    env.bootstrap()
    try:
        # wait for status info....
        try:
            try:
                env.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                env.get_status()
            env.wait_for_started()
        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        env.destroy_environment()
    if env.config['type'] == 'maas':
        sleep(90)
        for machine, running in running_domains.items():
            if not running:
                name, URI = machine.split('@')
                status_msg = stop_libvirt_domain(URI, name)
                print("%s" % status_msg)


def main():
    parser = ArgumentParser('Test a cloud')
    parser.add_argument('env', help='The juju environment to test')
    parser.add_argument('--machine', help='KVM machine to start.',
                        action='append', default=[])
    args = parser.parse_args()
    debug = bool(os.environ.get('DEBUG') == 'true')
    try:
        deploy_stack(args.env, debug, args.machine)
    except Exception as e:
        print('%s: %s' % (type(e), e))
        sys.exit(1)


if __name__ == '__main__':
    main()
