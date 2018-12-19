#!/usr/bin/env python

""" Assess using charms with lxd-profiles in different deployment scenarios.
"""

from __future__ import print_function

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
    WaitForLXDProfileCondition,
)

__metaclass__ = type

log = logging.getLogger("assess_lxdprofile_charm")
charm_profile_subordinate = 'lxd-profile-subordinate'
charm_profile_subordinate_path = 'charms/'+charm_profile_subordinate
charm_profile = 'lxd-profile'
charm_profile_path = 'charms/'+charm_profile
charm_ubuntu = 'ubuntu'
charm_ubuntu_path = 'charms/'+charm_ubuntu

def setup_principal(client, series, charm_principal_path):
    charm_sink = local_charm_path(
        charm=charm_principal_path,
        juju_ver=client.version,
        series=series,
        repository=os.environ['JUJU_REPOSITORY'])
    _, primary = client.deploy(charm_sink, series=series)
    client.wait_for(primary)

def setup_subordinate(client, series, charm_principal, charm_subordinate, charm_subordinate_path):
    charm_sink_2 = local_charm_path(
        charm=charm_subordinate_path,
        juju_ver=client.version,
        series=series,
        repository=os.environ['JUJU_REPOSITORY'])
    _, secondary = client.deploy(charm_sink_2, series=series)
    client.juju('add-relation', (charm_principal, charm_subordinate))
    client.wait_for_subordinate_units(charm_principal, charm_subordinate)

def assess_juju_lxdprofile_machine_upgrade(client, charm_principal, verify_principal, unit_num, charm_subordinate):
    """ Tests juju status and lxd profiles

    Verify with juju status
    Upgrade the both charms
    Verify new lxd-profiles in juju status

    Assumes that it's acting on principal unit 0
    Assumes only 1 subordinate
    """

    lxdprofile_machine_verify(client, charm_principal, verify_principal, unit_num)

    repository = os.environ['JUJU_REPOSITORY']
    client.upgrade_charm(charm_principal, repository+"/charms/"+charm_principal)
    client.wait_for(AgentsIdle([charm_principal+"/"+unit_num]))

    if verify_principal:
        lxdprofile_machine_verify(client, charm_principal, verify_principal,unit_num)

    principal_unit = client.get_status().get_unit(charm_principal+"/"+unit_num)
    subordinate_unit = principal_unit['subordinates'].keys()[0]
    client.upgrade_charm(charm_subordinate, repository+"/charms/"+charm_subordinate)
    client.wait_for(AgentsIdle([subordinate_unit]))

    lxdprofile_machine_verify(client, charm_principal, verify_principal, unit_num)

def lxdprofile_machine_verify(client, charm_name, verify_principal, unit_num):
    """ Checks the status output is the same as derived profile name.

    :param client: Juju client
    :param charm_name: LXD Profile name to expect in the output, principal units only
    :param verify_principal: Should the principal charm's lxd profile be verified?
    :param unit_num: Which unit of the principal charm to check
    :return: None
    :raises JujuAssertionError: if lxd profile is not appropriately found.
    """
    status = client.get_status()

    unit_info = status.get_unit(charm_name+"/"+unit_num)
    try:
        machine_num = unit_info['machine']
    except KeyError:
        log.warning("lxdprofile_machine_verify called on subordinate {}/{}, invalid".format(charm_name, unit_num))
        return

    application_info = status.get_applications()
    charm_rev = application_info[charm_name]['charm-rev']
    profile_name = "juju-{}-{}-{}".format(client.model_name,charm_name,charm_rev)

    if verify_principal:
        client.wait_for(WaitForLXDProfileCondition(machine_num, profile_name))

    # check subordinates, do their profile names match the charm rev?
    if 'subordinates' in unit_info:
        for key in unit_info['subordinates'].keys():
            sub_app_name = key.split('/')[0]
            sub_charm_rev = application_info[sub_app_name]['charm-rev']
            sub_profile_name = "juju-{}-{}-{}".format(client.model_name,sub_app_name,sub_charm_rev)
            client.wait_for(WaitForLXDProfileCondition(machine_num, sub_profile_name))

    log.info("juju machine {} is using {}: verification succeeded".format(machine_num,profile_name))

def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju lxd profile deploy.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client

        setup_principal(client, args.series, charm_profile_path)
        setup_subordinate(client, args.series, charm_profile, charm_profile_subordinate, charm_profile_subordinate_path)
        assess_juju_lxdprofile_machine_upgrade(client, charm_profile, True, "0", charm_profile_subordinate)

        setup_principal(client, args.series, charm_ubuntu_path)
        client.juju('add-relation', (charm_ubuntu, charm_profile_subordinate))
        client.wait_for_subordinate_units(charm_ubuntu, charm_profile_subordinate)
        assess_juju_lxdprofile_machine_upgrade(client, charm_ubuntu, False,"0",  charm_profile_subordinate)

        client.juju('add-unit', charm_profile)
        client.juju('add-unit', (charm_profile, '--to', 'lxd'))
        client.wait_for_started()
        client.wait_for_subordinate_units(charm_profile, charm_profile_subordinate)
        assess_juju_lxdprofile_machine_upgrade(client, charm_profile, True, "1", charm_profile_subordinate)
    return 0

if __name__ == '__main__':
    sys.exit(main())
