from argparse import ArgumentParser
from utility import add_basic_testing_arguments


def get_base_parser(description=""):
    """Return a parser with the base arguments required
    for any asses_ test.

    :param description: description for argument parser.
    :type description: str
    :returns: argument parser with base fields
    :rtype: ArgumentParser
    """
    parser = ArgumentParser(description)
    parser = add_basic_testing_arguments(parser)
    parser.add_argument(
        'juju_path', help='Directory your juju binary lives in.')
    return parser


class AssertError(Exception):
    """An assert function failed"""


class AssertFailed(Exception):
    """An assert failed"""


def assertion_test(assertion):
    """assertion_test is a decorator to use on bool returning assert
    functions from our tests, it is just for sintactic sugar purposes
    """
    def wrap_assertion(*args, **kwargs):
        result = assertion(*args, **kwargs)
        if result is not True and result is not False:
            raise AssertError("Expected bool, obtained \"%s\"" % result)
        if result:
            return
        raise AssertFailed("%s failed: %s" % (assertion.__name__, result))
    return wrap_assertion
