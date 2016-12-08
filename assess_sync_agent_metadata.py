#!/usr/bin/env python
"""python assess_sync_agent_metadata.py --agent-metadata-url=~/stream"""

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

__metaclass__ = type


log = logging.getLogger("assess_sync_agent_metadata")


def verify_deployed_charm(client):
    remote = remote_from_unit(client, "ubuntu/0")
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


def assess_deploy_charm(client):
    try:
        ubuntu_charm = "cs:xenial/ubuntu"
        client.deploy(ubuntu_charm)
        client.wait_for_started()
        client.wait_for_workloads()
        log.info("Deployed {} charm successfully".format(ubuntu_charm))
        import pdb
        pdb.set_trace()
        verify_deployed_charm(client)
    except Exception as e:
        logging.exception(e)


def assess_sync_bootstrap(args, agent_stream="release"):
    """
        Do sync-tool and then perform juju bootstrap with
        metadata_source and agent-metadata-url option.
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    client.env.update_config(
        {'agent-metadata-url': args.agent_metadata_url,
         'agent-stream:': agent_stream})

    with prepare_temp_metadata(client, args.agent_metadata_url) \
            as metadata_dir:
        log.info('Metadata written to: {}'.format(metadata_dir))
        with thin_booted_context(bs_manager,
                                 metadata_source=metadata_dir):
            log.info('Metadata bootstrap successful.')
            assess_check_metadata(args, client)
            verify_deployed_tool(args.agent_metadata_url, client)
            assess_deploy_charm(client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test agent-metadata on sync-tool")
    add_basic_testing_arguments(parser)
    parser.add_argument('--agent-metadata-url', required=True,
                        action='store', default=None,
                        help='Directory to store metadata.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    assess_sync_bootstrap(args)
    assess_sync_bootstrap(args, agent_stream="devel")
    return 0


if __name__ == '__main__':
    sys.exit(main())
