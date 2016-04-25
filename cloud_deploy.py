#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import os
import subprocess
import sys
from time import sleep

from jujupy import (
    CannotConnectEnv,
    EnvJujuClient,
    SimpleEnvironment,
)
from substrate import (
    LIBVIRT_DOMAIN_RUNNING,
    start_libvirt_domain,
    stop_libvirt_domain,
    verify_libvirt_domain,
)
from utility import local_charm_path


__metaclass__ = type


def deploy_stack(environment, debug, machines, deploy_charm):
    """"Deploy a test stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(environment), debug=debug)
    running_domains = dict()
    if client.env.config['type'] == 'maas':
        # Split the hypervisor_URI and machine name
        for machine in machines:
            name, URI = machine.split('@')
            # Record already running domains, so they can be left running,
            # if already running; otherwise start them.
            if verify_libvirt_domain(URI, name, LIBVIRT_DOMAIN_RUNNING):
                print("%s is already running" % name)
                running_domains = {machine: True}
            else:
                running_domains = {machine: False}
                print("Attempting to start %s at %s" % (name, URI))
                status_msg = start_libvirt_domain(URI, name)
                print("%s" % status_msg)
    # Clean up any leftover junk
    client.destroy_environment()
    client.bootstrap()
    try:
        # wait for status info....
        try:
            try:
                client.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                client.get_status()
            client.wait_for_started()
            if deploy_charm:
                series = client.env.config.get('default-series', 'trusty')
                charm_path = local_charm_path(
                    'dummy-source', juju_ver=client.version, series=series)
                client.deploy(charm_path, series=series)
                client.wait_for_started()
        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        client.destroy_environment()
        if client.env.config['type'] == 'maas':
            sleep(90)
            for machine, running in running_domains.items():
                name, URI = machine.split('@')
                if running:
                    print("WARNING: %s at %s was running when deploy_job "
                          "started. Shutting it down to ensure a clean "
                          "environment."
                          % (name, URI))
                status_msg = stop_libvirt_domain(URI, name)
                print("%s" % status_msg)


def main():
    parser = ArgumentParser('Test a cloud')
    parser.add_argument('env', help='The juju environment to test')
    parser.add_argument('--machine', help='KVM machine to start.',
                        action='append', default=[])
    parser.add_argument('--deploy-charm', action='store_true')
    args = parser.parse_args()
    debug = bool(os.environ.get('DEBUG') == 'true')
    try:
        deploy_stack(args.env, debug, args.machine, args.deploy_charm)
    except Exception as e:
        print('%s: %s' % (type(e), e))
        sys.exit(1)


if __name__ == '__main__':
    main()
