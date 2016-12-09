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
from ast import literal_eval
from contextlib import contextmanager
from hashlib import sha256
from shutil import rmtree
from textwrap import dedent
from time import sleep

import io
import logging
import os
import subprocess
import sys
import yaml

from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
)

JUJU_TOOL_PATH = "/tools/released/"
JUJU_STREAM_PATH = "/tools/streams/"

test_cloud_yaml = "testCloud.yaml"
test_cloud_name = "testCloud"

log = logging.getLogger("assess_agent_metadata")


def get_sha256_sum(filename):
    try:
        with open(filename, 'rb') as infile:
            return sha256(infile.read()).hexdigest()
    except Exception as e:
        logging.exception(e)


def assess_check_cloud(client, cloud_name, example_cloud):
    clouds = client.env.read_clouds()
    if len(clouds['clouds']) == 0:
        raise JujuAssertionError('Clouds missing!')
    if cloud_name not in clouds['clouds'].keys():
        raise JujuAssertionError(
            'Name mismatch and Cloud {} missing'.format(cloud_name))
    if clouds['clouds'][cloud_name] != example_cloud:
        sys.stderr.write('\nExpected:\n')
        yaml.dump(example_cloud, sys.stderr)
        sys.stderr.write('\nActual:\n')
        yaml.dump(clouds['clouds'][cloud_name], sys.stderr)
        raise JujuAssertionError('Cloud mismatch')


def iter_clouds(clouds):
    for cloud_name, cloud in clouds.items():
        yield (cloud_name, cloud)


def get_local_url_and_sha256(agent_dir, controller_url):
    """
    Get the agent URL (local file location: file:///)
    and SHA256 of the image-file passed
    """
    controller_agent_file = os.path.basename(controller_url)
    local_url = agent_dir + JUJU_TOOL_PATH + controller_agent_file

    if not os.path.isfile(local_url):
        raise JujuAssertionError(
            "File not found {}".format(local_url))

    local_sha256 = get_sha256_sum(local_url)
    local_url = "file://" + local_url
    return dict(URL=local_url, SHA256=local_sha256)


def get_controller_url_and_sha256(client):
    """
    Get the agent url and sha256 of the controller.
    """
    controller_client = client.get_controller_client()
    try:
        output = controller_client.run(
            ['cat /var/lib/juju/tools/machine-0/downloaded-tools.txt'],
            machines=['0'])
        output_ = literal_eval(output[0]['Stdout'])
        if (output_['url']) and (output_['sha256']):
                return [output_['url'], output_['sha256']]
    except Exception as e:
        logging.exception(e)


def assess_check_metadata(agent_dir, client):
    data = client.get_model_config()
    if agent_dir != data['agent-metadata-url']['value']:
        raise JujuAssertionError('Error, mismatch agent-metadata-url')

    log.info('bootstrap successfully with agent-metdadata-url={}'
             .format(data['agent-metadata-url']['value']))


def verify_deployed_tool(agent_dir, client):
    controller_url, controller_sha256 = \
        get_controller_url_and_sha256(client)

    if not controller_url and controller_sha256:
        raise JujuAssertionError('Failed to get controller URL and SHA256')

    log.info("controller_url: {} and controller_sha256: {}"
             .format(controller_url, controller_sha256))

    local_url_sha256 = get_local_url_and_sha256(
        agent_dir, controller_url)

    if not local_url_sha256:
        raise JujuAssertionError('Failed to get local URL and SHA256')

    log.info("local_url: {} and local_sha256: {}"
             .format(local_url_sha256["URL"], local_url_sha256["SHA256"]))

    if local_url_sha256["URL"] != controller_url:
        raise JujuAssertionError(
            "mismatch local URL {} and controller URL {}".format
            (local_url_sha256["URL"], controller_url))

    if local_url_sha256["SHA256"] != controller_sha256:
        raise JujuAssertionError(
            "mismatch local SHA256 {} and controller SHA256 {}".format
            (local_url_sha256["SHA256"], controller_sha256))


def access_metadata(args):
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    client.env.update_config({'agent-metadata-url': args.agent_dir})
    log.info('bootstrap to use --agent_metadata_url={}'.
             format(args.agent_dir))
    client.generate_tool(args.agent_dir)
    try:
        with bs_manager.booted_context(args.upload_tools):
            log.info('Metadata bootstrap successful.')
            assess_check_metadata(args.agent_dir, client)
            verify_deployed_tool(args.agent_dir, client)
            log.info("Successfully deployed and verified agent-metadata-url")
    except Exception as e:
        logging.exception(e)


def do_add_cloud(args, clouds):
    for cloud_name, cloud in iter_clouds(clouds):
        bs_manager = BootstrapManager.from_args(args)
        client = bs_manager.client
        client.add_cloud(cloud_name, test_cloud_yaml)
        assess_check_cloud(client, cloud_name, cloud)
        with bs_manager.booted_context(args.upload_tools):
            log.info('Metadata bootstrap successful.')
            verify_deployed_tool(args.agent_dir, client)
            log.info("Successfully deployed and verified add-cloud")


def create_add_cloud_yaml_file(agent_metadata_url):
    testCloud_yaml_data = dedent("""\
        clouds:
            {0}:
                type: lxd
                config:
                     agent-metadata-url: file://{1}
        """).format(test_cloud_name, agent_metadata_url)
    try:
        with io.FileIO(test_cloud_yaml, "w") as infile:
            infile.write(testCloud_yaml_data)
    except Exception as e:
        logging.exception(e)


def access_add_cloud(args):
    create_add_cloud_yaml_file(args.agent_dir)
    try:
        with open(test_cloud_yaml) as f:
            clouds = yaml.safe_load(f)['clouds']
            do_add_cloud(args, clouds)
    except Exception as e:
        logging.exception(e)

    if os.path.isfile(test_cloud_yaml):
        os.remove(test_cloud_yaml)


def append_empty_string(src, dst):
    """
        Append empty string to the tgz file so that the SHA256 sum of the file
        get modified. We use this to make sure that controller deployed on
        bootstrap used of the changed tgz file.

    :param src: The source tgz file
    :param dst: The destination tgz file
    :return: None
    """
    if not src.endswith(".tgz"):
        pass
    try:
        command = "cat {}  <(echo -n ''| gzip)> {}".format(src, dst)
        subprocess.Popen(command, shell=True, executable='/bin/bash')
    except subprocess.CalledProcessError as e:
        raise JujuAssertionError(
            "Failed to change tool file {} - {}". format(src, e))


@contextmanager
def make_unique_tool(args):
    """
        Create a tool directory for juju agent tools and stream and invoke
        append_empty_string function for the agent-file passed.

    :param args:
    :return: None
    """
    juju_tool_src = args.agent_dir + JUJU_TOOL_PATH
    juju_stream_dst = args.agent_dir + JUJU_STREAM_PATH
    try:
        if not os.path.exists(juju_tool_src):
            os.makedirs(juju_tool_src)

        if not os.path.exists(juju_stream_dst):
            os.makedirs(juju_stream_dst)

        append_empty_string(args.agent_file,
                            juju_tool_src + os.path.basename(args.agent_file))
        # sleep for a while to avoid broken pipe error
        sleep(2)
        yield juju_tool_src
    except Exception as e:
        logging.exception(e)
    finally:
        rmtree(args.agent_dir)


def parse_args(argv):
    """Parse all arguments."""
    parser = ArgumentParser(
        description="Test bootstrap for agent-metdadata-url")

    add_basic_testing_arguments(parser)

    parser.add_argument('--agent-file',
                        action='store', default=None,
                        help='agent file to be used during bootstrap.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    if not os.path.isfile(args.agent_file):
        raise JujuAssertionError(
            "Unable to find image-file {}".format(args.agent_file))

    """
        This test case will do juju bootstrap with agent-metadata-url
        option.
    """
    with temp_dir() as args.agent_dir:
        with make_unique_tool(args):
            access_metadata(args)

    """
        This test case will perform juju bootstrap on add-cloud.
        The add-cloud yaml file will define the agent-metadata-url
        option.
    """

    with temp_dir() as args.agent_dir:
        with make_unique_tool(args):
            access_add_cloud(args)

    return 0


if __name__ == '__main__':
    sys.exit(main())
