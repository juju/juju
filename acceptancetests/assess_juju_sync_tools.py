#!/usr/bin/env python3

from __future__ import print_function

import argparse
import logging
import sys
import os

from assess_bootstrap import (
    prepare_temp_metadata,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )
from jujupy import (
    client_from_config,
    )

__metaclass__ = type


log = logging.getLogger("assess_juju_sync_tools")


def assert_file_version_matches_agent_version(agent_file, agent_version):
    """Verify the agent_file start with the specified agent version.

    :param agent_file: String representing agent file name
    :param agent_version: String representing agent version
    """
    agent_file_parts = agent_file.split('-')
    if len(agent_file_parts) == 4:
        agent_file_version = '-'.join(agent_file_parts[1:2])
    if len(agent_file_parts) == 5:
        agent_file_version = '-'.join(agent_file_parts[1:3])

    if agent_file_version != agent_version:
        raise JujuAssertionError(
            "Mismatch agent file {} version found. Expected version {}".format(
                agent_file, agent_version))


def verify_agent_tools(agent_dir, agent_version):
    """Verify all the files in agent directory were of specific agent version

    :param agent_dir: String representing agent directory path
    :param agent_version: String representing agent version
    """
    agent_files = [f for f in os.listdir(agent_dir) if f.endswith('.tgz')]
    for agent_file in agent_files:
        assert_file_version_matches_agent_version(
            agent_file, agent_version)
    log.info('juju sync-tool verification done successfully')


def assess_juju_sync_tools(client, agent_stream, agent_version):
    """ Do sync-tool and verify the downloaded agent version.

    :param client: ModelClient juju
    :param agent_stream: String representing agent stream
    :param agent_version: String representing agent version
    """
    source = client.env.get_option('tools-metadata-url')
    with prepare_temp_metadata(
            client, None, agent_stream, source) as top_agent_dir:
        agent_dir = os.path.join(top_agent_dir, "tools", agent_stream)
        verify_agent_tools(agent_dir, agent_version)


def get_agent_version(client):
    """Get juju agent version

    :param client: ModelClient juju
    :return: The juju agent version
    """
    agent_version = client.get_matching_agent_version()
    return agent_version


def parse_args(argv):
    """Parse all arguments."""

    parser = argparse.ArgumentParser(
        description="Testing sync tools operates correctly")
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    client = client_from_config(args.env, args.juju_bin, False)
    agent_stream = args.agent_stream if args.agent_stream else 'devel'
    agent_version = get_agent_version(client)
    assess_juju_sync_tools(client, agent_stream, agent_version)
    return 0


if __name__ == '__main__':
    sys.exit(main())
