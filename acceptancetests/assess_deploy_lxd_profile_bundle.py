#!/usr/bin/env python

""" Assess using bundle that have various charms with lxd-profiles, testing
    different deployment scenarios.
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
)
from jujupy.wait_condition import (
    AgentsIdle,
    WaitForLXDProfilesConditions,
)

__metaclass__ = type

log = logging.getLogger("assess_lxdprofile_charm")

default_bundle = 'bundles-lxd-profile.yaml'

def deploy_bundle(client, charm_bundle):
    """Deploy the given charm bundle
    :param client: Jujupy ModelClient object
    :param charm_bundle: Optional charm bundle string
    """
    if not charm_bundle:
        bundle = local_charm_path(
            charm=default_bundle,
            juju_ver=client.version,
            repository=os.environ['JUJU_REPOSITORY']
        )
    else:
        bundle = charm_bundle
    _, primary = client.deploy(bundle)
    client.wait_for(primary)

def assess_profile_machines(client):
    """Assess the machines
    """
    # Ensure we wait for everything to start before checking the profiles,
    # that way we can ensure that things have been installed.
    client.wait_for_started()

    charm_profiles = []
    status = client.get_status()
    apps = status.get_applications()
    for _, info in apps.items():
        if 'charm-profile' in info:
            charm_profile = info['charm-profile']
            if charm_profile:
                charm_profiles.append(charm_profile)
    if len(charm_profiles) > 0:
        client.wait_for(WaitForLXDProfilesConditions(charm_profiles))

def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju lxd profile bundle deploys.")
    parser.add_argument(
        '--charm-bundle',
        help="Override the charm bundle to deploy",
    )
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client

        deploy_bundle(client, charm_bundle=args.charm_bundle)
        assess_profile_machines(client)

if __name__ == '__main__':
    sys.exit(main())
