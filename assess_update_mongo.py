#!/usr/bin/env python
"""Test juju update-mongo command."""

from __future__ import print_function

__metaclass__ = type

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from remote import remote_from_address
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


log = logging.getLogger("assess_update_mongo")

# The synlinks are shims while we wait for new packaging.
DEP_SCRIPT = """\
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y software-properties-common
sudo apt-add-repository -y ppa:juju/experimental
sudo apt-get update
sudo apt-get install -y juju-mongodb2.6 juju-mongodb3.2 juju-mongo-tools3.2
sudo ln -s /usr/lib/juju/mongodb2.6 /usr/lib/juju/mongo2.6
sudo ln -s /usr/lib/juju/mongodb3.2 /usr/lib/juju/mongo3
"""


def assess_update_mongo(client, series, bootstrap_host):
    charm = 'local:{}/ubuntu'.format(series)
    log.info("Setting up test.")
    client.deploy(charm)
    client.wait_for_started()
    log.info("Setup complete.")
    log.info("Test started.")
    # Instrument the case where Juju can install the new mongo packages from
    # Ubuntu.
    remote = remote_from_address(bootstrap_host, series=series)
    remote.run(DEP_SCRIPT)
    client.upgrade_mongo()
    # Wait for upgrade
    # Verify mongo 3 runs on the server
    # Check status.
    log.info("Test complete.")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test juju update-mongo command")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_update_mongo(
            bs_manager.client, args.series, bs_manager.bootstrap_host)
    return 0


if __name__ == '__main__':
    sys.exit(main())
