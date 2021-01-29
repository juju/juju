#!/usr/bin/env python3

""" Assess upgrading charms with lxd-profiles in different deployment scenarios.
"""

import argparse
import logging
import os
import sys

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    is_subordinate,
    subordinate_machines_from_app_info,
    application_machines_from_app_info,
    align_machine_profiles,
)
from jujupy.wait_condition import (
    WaitForLXDProfilesConditions,
)

__metaclass__ = type

log = logging.getLogger("assess_lxdprofile_charm")

default_bundle = 'bundles-lxd-profile-upgrade.yaml'

def deploy_bundle(client):
    """Deploy the given charm bundle
    :param client: Jujupy ModelClient object
    """
    bundle = local_charm_path(
        charm=default_bundle,
        juju_ver=client.version,
        repository=os.environ['JUJU_REPOSITORY']
    )
    _, primary = client.deploy(bundle)
    client.wait_for(primary)

def upgrade_charm(client):
    client.upgrade_charm("lxd-profile", revision='3')

def assess_profile_machines(client):
    """Assess the machines
    """
    # Ensure we wait for everything to start before checking the profiles,
    # that way we can ensure that things have been installed.
    client.wait_for_started()

    machine_profiles = []
    status = client.get_status()
    apps = status.get_applications()
    for _, info in apps.items():
        if 'charm-profile' in info:
            charm_profile = info['charm-profile']
            if charm_profile:
                if is_subordinate(info):
                    machines = subordinate_machines_from_app_info(info, apps)
                else:
                    machines = application_machines_from_app_info(info)
                machine_profiles.append((charm_profile, machines))
    if len(machine_profiles) > 0:
        aligned_machine_profiles = align_machine_profiles(machine_profiles)
        client.wait_for(WaitForLXDProfilesConditions(aligned_machine_profiles))

def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju lxd profile bundle deploys.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client

        deploy_bundle(client)
        assess_profile_machines(client)
        upgrade_charm(client)
        assess_profile_machines(client)

if __name__ == '__main__':
    sys.exit(main())
