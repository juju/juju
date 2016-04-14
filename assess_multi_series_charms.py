#!/usr/bin/env python
"""
Charms have the capability to declare that they support more than
one  series. Previously a separate copy of the charm was required for
each  series. An important constraint here is that for a given charm,
all of the  listed series must be for the same distro/OS; it is not
allowed to offer a  single charm for Ubuntu and CentOS for example.
Supported series are added to charm metadata as follows:

    name: mycharm
    summary: "Great software"
    description: It works
    series:
       - trusty
       - precise
       - wily

The default series is the first in the list:

    juju deploy mycharm

should deploy a mycharm service running on trusty.

A different, non-default series may be specified:

    juju deploy mycharm --series precise

It is possible to force the charm to deploy using an unsupported series
(so long as the underlying OS is compatible):

    juju deploy mycharm --series xenial --force

"""
from __future__ import print_function

import argparse
from collections import namedtuple
import logging
import subprocess
import sys

from deploy_stack import (BootstrapManager, )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    make_charm,
    temp_dir,
)
from assess_min_version import JujuAssertionError
from assess_heterogeneous_control import check_series


__metaclass__ = type

log = logging.getLogger("assess_multi_series_charms")


Test = namedtuple("Test", ["series", "service", "force", "success", "machine"])


def assess_multi_series_charms(client):
    """
    Assess multi series charms.

    :param client: Juju client.
    :type client: jujupy.EnvJujuClient
    :return: None
    """
    tests = [
        Test(series="precise", service='test0', force=False, success=False,
             machine=None),
        Test(series=None, service='test1', force=False, success=True,
             machine='0'),
        Test(series="trusty", service='test2', force=False, success=True,
             machine='1'),
        Test(series="xenial", service='test3', force=False, success=True,
             machine='2'),
        Test(series="precise", service='test4', force=True, success=True,
             machine='3'),
    ]
    with temp_dir() as charm_dir:
        make_charm(charm_dir, series=['trusty', 'xenial'])
        for test in tests:
            log.info(
                "Assessing multi series charms: test: {} charm_dir:{}".format(
                    test, charm_dir))
            assert_deploy(client, test, charm_dir)
            if test.machine:
                check_series(client, machine=test.machine, series=test.series)


def assert_deploy(client, test, charm_dir):
    """
    Deploy a charm and assert a success or fail.

    :param client: Juju client
    :type client: jujupy.EnvJujuClient
    :param test: Deploy test data.
    :type  test: Test
    :param charm_dir:
    :type charm_dir: str
    :return: None
    """
    if test.success:
        client.deploy(charm=charm_dir, series=test.series,
                      service=test.service, force=test.force)
        client.wait_for_started()
    else:
        try:
            client.deploy(charm=charm_dir, series=test.series,
                          service=test.service, force=test.force)
        except subprocess.CalledProcessError:
            return
        raise JujuAssertionError('Assert deploy failed for {}'.format(test))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test multi series charm feature")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_multi_series_charms(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
