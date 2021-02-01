#!/usr/bin/env python3
"""
Charms have the capability to declare that they support more than
one series. Previously a separate copy of the charm was required for
each series. An important constraint here is that for a given charm,
all of the  listed series must be for the same distro/OS; it is not
allowed to offer a  single charm for Ubuntu and CentOS for example.
Supported series are added to charm metadata as follows:

    name: mycharm
    summary: "Great software"
    description: It works
    series:
       - xenial
       - trusty
       - angsty

The default series is the first in the list:

    juju deploy mycharm

should deploy a mycharm service running on trusty.

A different, non-default series may be specified:

    juju deploy mycharm --series xenial

It is possible to force the charm to deploy using an unsupported series
(so long as the underlying OS is compatible):

    juju deploy mycharm --series angsty --force

"""
from __future__ import print_function

import argparse
from collections import namedtuple
import logging
import os
import subprocess
import sys

from deploy_stack import BootstrapManager
from jujucharm import (
    Charm,
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
)
from assess_heterogeneous_control import check_series


__metaclass__ = type

log = logging.getLogger("assess_multi_series_charms")


Test = namedtuple("Test", ["series", "service", "force", "success", "machine",
                           "juju1x_supported"])


def assess_multi_series_charms(client, devel_series):
    """Assess multi series charms.

    :param client: Juju client.
    :param devel_series: The series to use for new and unsupported scenarios.
    :type client: jujupy.ModelClient
    :return: None
    """
    tests = [
        Test(series=devel_series, service='test0', force=False, success=False,
             machine=None, juju1x_supported=False),
        Test(series=None, service='test1', force=False, success=True,
             machine='0', juju1x_supported=True),
        Test(series="trusty", service='test2', force=False, success=True,
             machine='1', juju1x_supported=True),
        Test(series="xenial", service='test3', force=False, success=True,
             machine='2', juju1x_supported=False),
        Test(series="bionic", service='test4', force=False, success=True,
             machine='2', juju1x_supported=False),
        Test(series=devel_series, service='test5', force=True, success=True,
             machine='3', juju1x_supported=False),
    ]
    with temp_dir() as repository:
        charm_name = 'dummy'
        charm = Charm(charm_name, 'Test charm', series=['trusty', 'xenial', 'bionic'])
        charm_dir = charm.to_repo_dir(repository)
        charm_path = local_charm_path(
            charm=charm_name, juju_ver=client.version, series='trusty',
            repository=os.path.dirname(charm_dir))
        for test in tests:
            if client.is_juju1x() and not test.juju1x_supported:
                continue
            log.info(
                "Assessing multi series charms: test: {} charm_dir:{}".format(
                    test, charm_path))
            assert_deploy(client, test, charm_path, repository=repository)
            if test.machine:
                check_series(client, machine=test.machine, series=test.series)


def assert_deploy(client, test, charm_path, repository=None):
    """Deploy a charm and assert a success or fail.

    :param client: Juju client
    :type client: jujupy.ModelClient
    :param test: Deploy test data.
    :type  test: Test
    :param charm_dir:
    :type charm_dir: str
    :param repository: Direcotry path to the repository
    :type repository: str
    :return: None
    """
    if test.success:
        client.deploy(charm=charm_path, series=test.series,
                      service=test.service, force=test.force,
                      repository=repository)
        client.wait_for_started()
    else:
        try:
            client.deploy(charm=charm_path, series=test.series,
                          service=test.service, force=test.force,
                          repository=repository)
        except subprocess.CalledProcessError:
            return
        raise JujuAssertionError('Assert deploy failed for {}'.format(test))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test multi series charm feature")
    add_basic_testing_arguments(parser)
    parser.add_argument(
        '--devel-series', default="yakkety",
        help="The series to use when testing new and unsupported scenarios.")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_multi_series_charms(bs_manager.client, args.devel_series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
