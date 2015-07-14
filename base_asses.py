from argparse import ArgumentParser
from deploy_stack import add_juju_args, add_output_args
from utility import add_basic_testing_arguments
from unittest import FunctionTestCase


def get_base_parser(description=""):
    """Return a parser with the base arguments required
    for any asses_ test.

    :param description: description for argument parser.
    :type description: str
    :returns: argument parser with base fields
    :rtype: ArgumentParser
    """
    parser = ArgumentParser(description)
    add_basic_testing_arguments(parser)
    parser.add_argument(
        'juju_path', help='Directory your juju binary lives in.')
    parser.add_argument(
        'temp_env_name', help='An arbitrary name for this job.')

    add_juju_args(parser)
    add_output_args(parser)

    parser.add_argument('--bootstrap-host',
                        help='The host to use for bootstrap.', default=None)
    parser.add_argument('--machine', help='A machine to add or when used with '
                        'KVM based MaaS, a KVM image to start.',
                        action='append', default=[])
    parser.add_argument('--keep-env', action='store_true', default=False,
                        help='Keep the Juju environment after the test'
                        ' completes.')
    parser.add_argument(
        '--upload-tools', action='store_true', default=False,
        help='upload local version of tools before bootstrapping')
    return parser


def toUnitTest(testCase):
    def unitTestify(*args, **kwargs):
        tc = FunctionTestCase(testCase)
        largs = list(args)
        largs.insert(1, tc)
        args = tuple(largs)
        return testCase(*args, **kwargs)

    return unitTestify
