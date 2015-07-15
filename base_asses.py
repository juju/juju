from argparse import ArgumentParser
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
    return parser


def toUnitTest(testCase):
    def unitTestify(*args, **kwargs):
        tc = FunctionTestCase(testCase)
        largs = list(args)
        largs.insert(1, tc)
        args = tuple(largs)
        return testCase(*args, **kwargs)

    return unitTestify
