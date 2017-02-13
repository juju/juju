#!/usr/bin/env python

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
    ModelClient,
    )

__metaclass__ = type


log = logging.getLogger("assess_juju_sync_tools")


def assert_file_version_matches_agent_version(agent_file, agent_version):
    """
    Verify the agent_file start wit the specified agent version.
    :param agent_file: String representing agent file name
    :param agent_version: String representing agent version
    """
    if not agent_file.startswith(agent_version):
        raise JujuAssertionError(
            "Mismatch agent file {} version found. Expected version {}".format(
                agent_file, agent_version))


def verify_agent_tools(agent_dir, agent_stream, agent_version):
    """
    Verify all the files in agent directory of stream were of specific
    agent version
    :param agent_dir: The top level agent directory path
    :param agent_stream: String representing agent stream
    :param agent_version: String representing agent version
    """
    sync_tool_dir = os.path.join(agent_dir, "tools", agent_stream)
    for agent_file in os.listdir(sync_tool_dir):
        if agent_file.endswith(".tgz"):
            assert_file_version_matches_agent_version(
                agent_file, "juju-{}".format(agent_version))
    log.info('juju sync-tool verification done successfully')


def assess_juju_sync_tools(client, agent_stream, agent_version):
    """
    Do juju sync-tool and verify that downloaded agent-files were specific
    version that of juju bin.
    :param client: The Juju client
    :param agent_stream: String representing agent stream
    :param agent_version: String representing agent version
    """
    source = client.env.get_option('tools-metadata-url')
    with prepare_temp_metadata(
            client, None, agent_stream, source) as agent_dir:
        verify_agent_tools(agent_dir, agent_stream, agent_version)


def get_agent_version(juju_bin):
    """
    Get juju agent version
    :param juju_bin: The path to juju bin
    :return: The juju agent version
    """
    agent_version = ModelClient.get_version(juju_bin).split('-', 1)[0]
    return agent_version


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test to download juju agent using sync-tool and verify")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    client = client_from_config(args.env, args.juju_bin, False)
    agent_stream = args.agent_stream if args.agent_stream else 'devel'
    agent_version = get_agent_version(args.juju_bin)
    log.warning("Required juju sync-tool version {}".format(agent_version))
    assess_juju_sync_tools(client, agent_stream, agent_version)
    return 0


if __name__ == '__main__':
    sys.exit(main())
