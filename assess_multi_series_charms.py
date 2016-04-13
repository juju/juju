#!/usr/bin/env python
"""
Charms now have the capability to declare that they support more than
one  series. Previously a separate copy of the charm was required for
each  series. An important constraint here is that for a given charm,
all of the  listed series must be for the same distro/OS; it is not
allowed to offer a  single charm for Ubuntu and CentOS for example.
Supported series are added  to charm metadata as follows:

    name: mycharm
    summary: "Great software"
    description: It works
    categories:
       - databases
    series:
       - trusty
       - precise
       - wily

The default series is the first in the list:

    juju deploy mycharm

will deploy a mycharm service running on trusty.

A different, non-default series may be specified:

    juju deploy mycharm --series precise

It is possible to force the charm to deploy using an unsupported series
(so long as the underlying OS is compatible):

    juju deploy mycharm --series xenial --force

The 'force' option may also be required when upgrading charms. Consider
the  case where a service is initially deployed with a charm supporting
precise  and trusty. A new version of the charm is published which only
supports  trusty and xenial. For services deployed on precise, upgrading
to the newer  charm revision is allowed, but only using force (note the
use of  '--force-series' since upgrade-charm also supports '--force-
units'):

    juju upgrade-charm mycharm --force-series

"""

from __future__ import print_function

import argparse
import logging
import sys
from collections import namedtuple


from deploy_stack import (BootstrapManager, )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    make_charm,
)

__metaclass__ = type

log = logging.getLogger("assess_multi_series_charms")


Row = namedtuple("TestRow", "series, host, force, success")


def assess_multi_series_charms(client):
    table = [
        Row(["xenial", "trusty"], "xenial", False, True),
        Row(
            ["trusty", "precise"], "xenial", False, False),
        Row(
            ["xenial"], "precise", False, False),
        Row(
            ["xenial"], "precise", True, True),
    ]
    charm = make_charm(series=series)
    client.deploy(charm, service=_make_name(series))
    # Wait for the deployment to finish.
    client.wait_for_started()
    log.info("TODO: Add log line about any test")
    # TODO: Add specific functional testing actions here.


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test multi series charm feature")
    # TODO: Add additional positional arguments.
    add_basic_testing_arguments(parser)
    # TODO: Add additional optional arguments.
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
