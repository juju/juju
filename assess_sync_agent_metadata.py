#!/usr/bin/env python
"""
   Do juju bootstrap with agent-metadata-url option by doing sync-tools first.
   On doing bootstrap of controller then depoly dummy charm.
   Validation will be done to verify right agent-metadata-url is used during
   bootstrap and then deployed dummy charm made use of controller agent.

   Usage: python assess_sync_agent_metadata.py
"""

from __future__ import print_function

import argparse
import logging
import sys
import json

from assess_agent_metadata import (
    verify_deployed_tool,
    get_controller_url_and_sha256,
)

from assess_bootstrap import (
    prepare_temp_metadata,
    )

from deploy_stack import (
    BootstrapManager,
    )

from remote import (
    remote_from_unit,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )
from jujucharm import (
    local_charm_path,
)

__metaclass__ = type


log = logging.getLogger("assess_sync_agent_metadata")


def verify_deployed_charm(charm_app, client):
    """
    Verfiy the deployed charm, to make sure it used the same
    juju tool of the controller by verifying the sha256 sum
    :param charm_app: The app name that need to be verified
    :param client: juju client
    :return:
    """
    remote = remote_from_unit(client, "{0}/0".format(charm_app))
    output = remote.cat(
        "/var/lib/juju/tools/machine-0/downloaded-tools.txt")

    output_ = json.loads(output)
    _, controller_sha256 = get_controller_url_and_sha256(client)

    if output_['sha256'] != controller_sha256:
        raise JujuAssertionError('Error, mismatch agent-metadata-url')

    log.info("Charm verification done successfully")


def deploy_charm_and_verify(client):
    """
    Deploy dummy charm from local repository and
    verify it uses the specified agent-metadata-url option
    :param client: juju client
    :return:
    """
    charm_app = "dummy-sink"
    charm_source = local_charm_path(
        charm=charm_app, juju_ver=client.version)
    client.deploy(charm_source)
    client.wait_for_started()
    verify_deployed_charm(charm_app, client)


def assess_sync_bootstrap(args, agent_stream="release"):
    """
    Do sync-tool and then perform juju bootstrap with
    metadata_source and agent-metadata-url option.
    :param args: Parsed command line arguments
    :param agent_stream: choice of release or develop
    :return:
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client

    with prepare_temp_metadata(client, args.agent_dir) as agent_dir:
        client.env.update_config({'agent-stream:': agent_stream})
        log.info('Metadata written to: {}'.format(agent_dir))
        with bs_manager.booted_context(args.upload_tools,
                                       metadata_source=agent_dir):
            log.info('Metadata bootstrap successful.')
            verify_deployed_tool(agent_dir, client)
            deploy_charm_and_verify(client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test agent-metadata on sync-tool")
    parser.add_argument('--agent-dir',
                        action='store', default=None,
                        help='tool dir to be used during bootstrap.')
    add_basic_testing_arguments(parser)

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    assess_sync_bootstrap(args, agent_stream="release")
    assess_sync_bootstrap(args, agent_stream="devel")
    return 0


if __name__ == '__main__':
    sys.exit(main())
