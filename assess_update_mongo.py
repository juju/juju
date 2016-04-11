#!/usr/bin/env python
"""Test juju update-mongo command."""

from __future__ import print_function

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
    local_charm_path,
)


__metaclass__ = type


log = logging.getLogger("assess_update_mongo")

DEP_SCRIPT = """\
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y software-properties-common
sudo apt-add-repository -y ppa:juju/experimental
sudo apt-get update
"""

VERIFY_SCRIPT = """\
ps ax | grep 'mongo3/bin/mongod --dbpath /var/lib/juju/db' | grep -v grep
"""


def assess_update_mongo(client, series, bootstrap_host):
    log.info('series={}, bootstrap_host={}'.format(series, bootstrap_host))
    return_code = 1
    charm = local_charm_path(
        charm='ubuntu', juju_ver=client.version, series=series)
    log.info("Setting up test.")
    client.deploy(charm, series=series)
    client.wait_for_started()
    log.info("Setup complete.")
    log.info("Test started.")
    # Instrument the case where Juju can install the new mongo packages from
    # Ubuntu.
    remote = remote_from_address(bootstrap_host, series=series)
    remote.run(DEP_SCRIPT)
    # upgrade-mongo returns 0 if all is well. status will work but not
    # explicitly show that mongo3 is running.
    client.upgrade_mongo()
    client.show_status()
    log.info("Checking bootstrap host for mongo3:")
    mongo_proc = remote.run(VERIFY_SCRIPT)
    log.info(mongo_proc)
    if '--port 37017' in mongo_proc and '--replSet juju' in mongo_proc:
        return_code = 0
    log.info("Controller upgraded to MongoDB 3.")
    log.info("Test complete.")
    return return_code


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test juju update-mongo command")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    return_code = 1
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        return_code = assess_update_mongo(
            bs_manager.client, args.series, bs_manager.known_hosts['0'])
        log.info("Tearing down test.")
    log.info("Teardown complete.")
    if return_code == 0:
        log.info('TEST PASS')
    else:
        log.info('TEST FAIL')
    return return_code


if __name__ == '__main__':
    sys.exit(main())
