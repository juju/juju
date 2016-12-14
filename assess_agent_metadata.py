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
from shutil import rmtree
from textwrap import dedent
from tempfile import mkdtemp

import io
import logging
import os
import subprocess
import sys
import yaml
import json

from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

JUJU_TOOL_PATH = "tools/released/"
JUJU_STREAM_PATH = "tools/streams/"

test_cloud_yaml = "testCloud.yaml"
test_cloud_name = "testCloud"

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


def iter_clouds(clouds):
    for cloud_name, cloud in clouds.items():
        yield (cloud_name, cloud)


def get_local_url_and_sha256(agent_dir, controller_url):
    """
    Get the agent URL (local file location: file:///) and SHA256
    of the agent-file passed
    :param agent_dir: The top level directory location of agent file.
    :param controller_url: The controller used agent file url
    """
    local_url = os.path.join(agent_dir,
                             JUJU_TOOL_PATH, os.path.basename(controller_url))
    local_sha256 = get_sha256_sum(local_url).format(local_url)
    local_url = "file://" + local_url
    return dict(URL=local_url, SHA256=local_sha256)


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
    if (output_['url']) and (output_['sha256']):
        return [output_['url'], output_['sha256']]


def assert_metadata_are_correct(agent_dir, client):
    """
    Verfiy the client agent-metadata-url uses the specfied option
    :param agent_dir: The top level directory location of agent file.
    :param client: Juju client
    """
    data = client.get_model_config()
    if agent_dir != data['agent-metadata-url']['value']:
        raise JujuAssertionError('Error, mismatch agent-metadata-url')

    log.info('bootstrap successfully with agent-metdadata-url={}'
             .format(data['agent-metadata-url']['value']))


def verify_deployed_tool(agent_dir, client):
    """
    Verify the bootstraped controller make use of the the specified
    agent-metadata-url.
    :param agent_dir:  The top level directory location of agent file.
    :param client: Juju client
    """
    controller_url, controller_sha256 = \
        get_controller_url_and_sha256(client)

    log.debug("controller_url: {} and controller_sha256: {}".
              format(controller_url, controller_sha256))

    local_url_sha256 = get_local_url_and_sha256(
        agent_dir, controller_url)

    log.debug("local_url: {} and local_sha256: {}".
              format(local_url_sha256["URL"], local_url_sha256["SHA256"]))

    if local_url_sha256["URL"] != controller_url:
        raise JujuAssertionError(
            "mismatch local URL {} and controller URL {}".format
            (local_url_sha256["URL"], controller_url))

    if local_url_sha256["SHA256"] != controller_sha256:
        raise JujuAssertionError(
            "mismatch local SHA256 {} and controller SHA256 {}".format
            (local_url_sha256["SHA256"], controller_sha256))


def assess_metadata(args, agent_dir):
    """
    Bootstrap juju controller with agent-metadata-url option
    and verify that bootstraped controller make use of specified
    agent-metadata-url option.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    client.env.update_config({'agent-metadata-url': agent_dir})
    log.info('bootstrap to use --agent_metadata_url={}'.
             format(agent_dir))
    client.generate_tool(agent_dir)

    with bs_manager.booted_context(args.upload_tools):
        log.info('Metadata bootstrap successful.')
        assert_metadata_are_correct(agent_dir, client)
        verify_deployed_tool(agent_dir, client)
        log.info("Successfully deployed and verified agent-metadata-url")


def _assess_add_cloud(args, agent_dir, clouds):
    """
    Perform juju add-cloud using the yaml file and then bootstrap
    the environment on the created cloud and make sure that it make
    use of agent-metadata-url specified in yaml file.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    :param clouds: The clouds that needs to be bootstraped.
    """
    for cloud_name, cloud in iter_clouds(clouds):
        bs_manager = BootstrapManager.from_args(args)
        client = bs_manager.client
        client.add_cloud(cloud_name, test_cloud_yaml)
        assert_cloud_details_are_correct(client, cloud_name, cloud)
        with bs_manager.booted_context(args.upload_tools):
            log.info('Metadata bootstrap successful.')
            verify_deployed_tool(agent_dir, client)
            log.info("Successfully deployed and verified add-cloud")


def create_add_cloud_yaml_file(agent_metadata_url):
    """
    Create a yaml file to perform juju add-cloud
    with agent-metadata-url option
    :param agent_metadata_url: The file location of agent-metadata-url
    """
    test_cloud_yaml_data = dedent("""\
        clouds:
            {0}:
                type: lxd
                config:
                     agent-metadata-url: file://{1}
        """).format(test_cloud_name, agent_metadata_url)
    with io.FileIO(test_cloud_yaml, "w") as infile:
        infile.write(test_cloud_yaml_data)


def assess_add_cloud(args, agent_dir):
    """
    Perform juju add-cloud by creating a yaml file for cloud
    with agent-metadata-url option and bootstrap.
    :param args: Parsed command line arguments
    :param agent_dir: he top level directory location of agent file.
    """
    create_add_cloud_yaml_file(agent_dir)
    with open(test_cloud_yaml) as f:
        clouds = yaml.safe_load(f)['clouds']
        _assess_add_cloud(args, agent_dir, clouds)
    os.remove(test_cloud_yaml)


def change_tgz_sha256_sum(src, dst):
    """
    Append empty string to the tgz file so that the SHA256 sum of the file
    get modified. We use this to make sure that controller deployed on
    bootstrap used of the changed tgz file.
    :param src: The source tgz file
    :param dst: The destination tgz file
    """
    if not src.endswith(".tgz"):
        raise JujuAssertionError("requires tgz file in {}".format(src))
    try:
        command = "cat {}  <(echo -n ''| gzip)> {}".format(src, dst)
        subprocess.Popen(command, shell=True, executable='/bin/bash')
    except subprocess.CalledProcessError as e:
        raise JujuAssertionError(
            "Failed to change tool file {} - {}". format(src, e))


@contextmanager
def make_unique_tool(agent_file):
    """
    Create a tool directory for juju agent tools and stream and invoke
    append_empty_string function for the agent-file passed.
    :param agent_file:
    """
    agent_dir = mkdtemp()
    juju_tool_src = os.path.join(agent_dir, JUJU_TOOL_PATH)
    juju_stream_dst = os.path.join(agent_dir, JUJU_STREAM_PATH)
    try:
        if not os.path.exists(juju_tool_src):
            os.makedirs(juju_tool_src)

        if not os.path.exists(juju_stream_dst):
            os.makedirs(juju_stream_dst)

        change_tgz_sha256_sum(agent_file,
                              os.path.join(juju_tool_src,
                                           os.path.basename(agent_file)))
        log.debug("Created agent directory to perform bootstrap".
                  format(agent_dir))
        yield agent_dir
    except Exception as e:
        raise JujuAssertionError("failed on make_unique_tool {}".format(e))
    finally:
        rmtree(agent_dir)


def parse_args(argv):
    """Parse all arguments."""
    parser = ArgumentParser(
        description="Test bootstrap for agent-metdadata-url")

    add_basic_testing_arguments(parser)

    parser.add_argument('--agent-file', required=True,
                        action='store', default=None,
                        help='agent file to be used during bootstrap.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    if not os.path.isfile(args.agent_file):
        raise JujuAssertionError(
            "Unable to find juju agent file {}".format(args.agent_file))

    # This test case will do juju bootstrap with agent-metadata-url
    # option.

    with make_unique_tool(args.agent_file) as agent_dir:
        assess_metadata(args, agent_dir)

    # This test case will perform juju bootstrap after performing juju
    # add-cloud. The cloud yaml file will define the agent-metadata-url
    # option.

    with make_unique_tool(args.agent_file) as agent_dir:
        assess_add_cloud(args, agent_dir)

    return 0


if __name__ == '__main__':
    sys.exit(main())
