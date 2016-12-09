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
from ast import literal_eval

from assess_agent_metadata import (
    verify_deployed_tool,
    assess_check_metadata,
    get_controller_url_and_sha256,
)

from assess_bootstrap import (
    thin_booted_context,
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
    remote = remote_from_unit(client, "{0}/0".format(charm_app))
    if not remote:
        JujuAssertionError('Failed during remote connection')

    output = remote.cat("/var/lib/juju/tools/machine-0/downloaded-tools.txt")
    if not output:
        JujuAssertionError('Unbale to perform remote cat')

    output_ = literal_eval(output)
    _, controller_sha256 = get_controller_url_and_sha256(client)
    if not controller_sha256:
        JujuAssertionError('Unbale to get controller url and sha256')

    if output_['sha256'] != controller_sha256:
        JujuAssertionError('Error, mismatch agent-metadata-url')

    log.info("Charm verfication done successfully")
    return


def assess_deploy_charm(client):
    charm_app = "dummy-sink"
    charm_source = local_charm_path(
        charm=charm_app, juju_ver=client.version)
    client.deploy(charm_source)
    verify_deployed_charm(charm_app, client)
    return


def assess_sync_bootstrap(args, agent_stream="release"):
    """
        Do sync-tool and then perform juju bootstrap with
        metadata_source and agent-metadata-url option.
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client

    with prepare_temp_metadata(client, args.agent_dir) \
            as agent_dir:
        client.env.update_config(
            {'agent-metadata-url': agent_dir,
             'agent-stream:': agent_stream})
        log.info('Metadata written to: {}'.format(agent_dir))
        with thin_booted_context(bs_manager,
                                 metadata_source=agent_dir):
            log.info('Metadata bootstrap successful.')
            assess_check_metadata(agent_dir, client)
            verify_deployed_tool(agent_dir, client)
            assess_deploy_charm(client)
    return


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

    assess_sync_bootstrap(args)

    assess_sync_bootstrap(agent_stream="devel")
    return 0


if __name__ == '__main__':
    sys.exit(main())
