#!/usr/bin/env python
"""
 Juju agent-metadata-url validation on passing as an argument during
 juju bootstrap

 Juju agent-metadata-url validation using cloud definition yaml file to
 verify that agent-metadata-url specified in the yaml file is applied
 while running juju boostrap command.

 Usage: python assess_agent_metadata.py --agent-file=/path/to/juju-*.tgz

 Example: python assess_agent_metadata.py
                --agent-file=/home/juju/juju-2.0.1-xenial-amd64.tgz
"""

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
from hashlib import sha256

import logging
import os
import subprocess
import sys
import json

from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
)

from jujupy import (
    temp_yaml_file,
)


log = logging.getLogger("assess_agent_metadata")


def get_sha256_sum(filename):
    """
    Get SHA256 sum of the given filename
    :param filename: The filename
    """
    with open(filename, 'rb') as infile:
        return sha256(infile.read()).hexdigest()


def assert_cloud_details_are_correct(client, cloud_name, example_cloud):
    """
    Check juju add-cloud is performed succesffuly and it is available
    in the juju client.
    :param client: The juju client
    :param cloud_name: The name of the cloud added
    :param example_cloud: The content of the cloud
    """
    clouds = client.env.read_clouds()
    try:
        if clouds['clouds'][cloud_name] != example_cloud:
            raise JujuAssertionError('Cloud mismatch')
    except KeyError:
        raise JujuAssertionError('Cloud missing {}'.format(cloud_name))


def get_local_url_and_sha256(agent_dir, controller_url,
                             agent_stream):
    """
    Get the agent URL (local file location: file:///) and SHA256
    of the agent-file passed
    :param agent_dir: The top level directory location of agent file.
    :param controller_url: The controller used agent file url
    :param agent_stream: The agent stream
    """
    local_url = os.path.join(agent_dir, "tools/{}".format(agent_stream),
                             os.path.basename(controller_url))

    local_sha256 = get_sha256_sum(local_url)
    local_file_path = "file://" + local_url
    return [local_file_path, local_sha256]


def get_controller_url_and_sha256(client):
    """
    Get the agent url and sha256 of the launched client
    :param client: Juju client
    """
    controller_client = client.get_controller_client()
    output = controller_client.run(
        ['cat /var/lib/juju/tools/machine-0/downloaded-tools.txt'],
        machines=['0'])
    output_ = json.loads(output[0]['Stdout'])
    return [output_['url'], output_['sha256']]


def assert_metadata_are_correct(agent_metadata_url, client):
    """
    verify the client agent-metadata-url uses the specified value
    :param agent_metadata_url: The agent file path.
    :param client: Juju client
    """
    data = client.get_model_config()
    if agent_metadata_url != data['agent-metadata-url']['value']:
        raise JujuAssertionError('Error, mismatch agent-metadata-url')

    log.info('bootstrap successfully with agent-metdadata-url={}'
             .format(data['agent-metadata-url']['value']))


def verify_deployed_tool(agent_dir, client, agent_stream):
    """
    Verify the bootstraped controller make use of the the specified
    agent-metadata-url.
    :param agent_dir:  The top level directory location of agent file.
    :param client: Juju client
    :param agent_stream: The agent stream
    """
    controller_url, controller_sha256 = get_controller_url_and_sha256(client)

    log.debug("controller_url: {} and controller_sha256: {}".format(
               controller_url, controller_sha256))

    local_url, local_sha256 = get_local_url_and_sha256(
        agent_dir, controller_url, agent_stream)

    log.debug("local_url: {} and local_sha256: {}".format(
        local_url, local_sha256))

    if local_url != controller_url:
        raise JujuAssertionError(
            "mismatch local URL {} and controller URL {}".format(
                local_url, controller_url))

    if local_sha256 != controller_sha256:
        raise JujuAssertionError(
            "mismatch local SHA256 {} and controller SHA256 {}".format(
                local_sha256, controller_sha256))


def assess_metadata(args, agent_dir):
    """
    Bootstrap juju controller with agent-metadata-url value
    and verify that bootstraped controller make use of specified
    agent-metadata-url value.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    agent_metadata_url = os.path.join(agent_dir, "tools/")

    client.env.discard_option('tools-metadata-url')
    client.env.update_config(
        {
            'agent-metadata-url': agent_metadata_url,
            'agent-stream': args.agent_stream
        }
    )
    log.info('bootstrap to use --agent_metadata_url={}'.format(
        agent_metadata_url))
    client.generate_tool(agent_dir, args.agent_stream)
    log.info("Directory contents {} with stream {}".format(
        agent_dir, args.agent_stream))
    list_files(agent_dir)

    log_dir = os.path.join(bs_manager.log_dir, 'assess_metadata')
    os.mkdir(log_dir)
    bs_manager.log_dir = log_dir

    with bs_manager.booted_context(args.upload_tools):
        log.info('Metadata bootstrap successful.')
        assert_metadata_are_correct(agent_metadata_url, client)
        verify_deployed_tool(agent_dir, client, args.agent_stream)
        log.info("Successfully deployed and verified agent-metadata-url")


def assess_add_cloud(args, agent_dir):
    """
    Perform juju add-cloud by creating a yaml file for cloud
    with agent-metadata-url value and bootstrap the juju environment.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    """

    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    agent_metadata_url = os.path.join(agent_dir, "tools/")

    cloud_name = client.env.get_cloud()
    # Remove the tool metadata url from the config (note the name, is
    # for historic reasons)
    client.env.discard_option('tools-metadata-url')
    cloud_details = {
        'clouds': {
            cloud_name: {
                'type': client.env.provider,
                'regions': {client.env.get_region(): {}},
                'config': {
                    'agent-metadata-url': 'file://{}'.format(
                        agent_metadata_url),
                    'agent-stream': args.agent_stream,
                }
            }
        }
    }

    with temp_yaml_file(cloud_details) as new_cloud:
        client.add_cloud(cloud_name, new_cloud)
        # Need to make sure we've refreshed any cache that we might have (as
        # this gets written to file during the bootstrap process.
        client.env.load_yaml()
        clouds = cloud_details['clouds'][cloud_name]
        assert_cloud_details_are_correct(client, cloud_name, clouds)

    client.generate_tool(agent_dir, args.agent_stream)
    list_files(agent_dir)

    log_dir = os.path.join(bs_manager.log_dir, 'assess_add_cloud')
    os.mkdir(log_dir)
    bs_manager.log_dir = log_dir
    with bs_manager.booted_context(args.upload_tools):
        log.info('Metadata bootstrap successful.')
        verify_deployed_tool(agent_dir, client, args.agent_stream)
        log.info("Successfully deployed and verified add-cloud")


def clone_tgz_file_and_change_shasum(original_tgz_file, new_tgz_file):
    """
    Create a new tgz file from the original tgz file and then add empty file
    to it so that the sha256 sum of the new tgz file will be different from
    that of original tgz file. We use this to make sure that controller
    deployed on bootstrap used of the new tgz file.
    :param original_tgz_file: The source tgz file
    :param new_tgz_file: The destination tgz file
    """
    if not original_tgz_file.endswith(".tgz"):
        raise Exception("{} is not tgz file".format(original_tgz_file))
    try:
        command = "cat {}  <(echo -n ''| gzip)> {}".format(
            original_tgz_file, new_tgz_file)
        subprocess.Popen(command, shell=True, executable='/bin/bash')
    except subprocess.CalledProcessError as e:
        raise Exception("Failed to create a tool file {} - {}".format(
            original_tgz_file, e))


def list_files(startpath):
    for root, dirs, files in os.walk(startpath):
        level = root.replace(startpath, '').count(os.sep)
        indent = ' ' * 4 * (level)
        log.info('{}{}/'.format(indent, os.path.basename(root)))
        subindent = ' ' * 4 * (level + 1)
        for f in files:
            log.info('{}{}'.format(subindent, f))


@contextmanager
def make_unique_tool(agent_file, agent_stream):
    """
    Create a tool directory for juju agent tools and stream and invoke
    clone_tgz_file_and_change_shasum function for the agent-file passed.
    :param agent_file: The agent file to use
    :param agent_stream: The agent stream to use
    """
    with temp_dir() as agent_dir:
        juju_tool_src = os.path.join(agent_dir, "tools/{}".format(
            agent_stream))
        os.makedirs(juju_tool_src)
        clone_tgz_file_and_change_shasum(agent_file, os.path.join(
            juju_tool_src, os.path.basename(agent_file)))
        log.debug("Created agent directory to perform bootstrap".format(
            agent_dir))
        yield agent_dir


def parse_args(argv):
    """Parse all arguments."""
    parser = ArgumentParser(
        description="Test bootstrap for agent-metdadata-url")

    add_basic_testing_arguments(parser)

    parser.add_argument('--agent-file', required=True, action='store',
                        help='agent file to be used during bootstrap.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    if not os.path.isfile(args.agent_file):
        raise Exception(
           "Unable to find juju agent file {}".format(args.agent_file))

    args.agent_stream = args.agent_stream if args.agent_stream else 'testing'

    with make_unique_tool(args.agent_file, args.agent_stream) as agent_dir:
        assess_metadata(args, agent_dir)

    with make_unique_tool(args.agent_file, args.agent_stream) as agent_dir:
        assess_add_cloud(args, agent_dir)

    return 0


if __name__ == '__main__':
    sys.exit(main())
