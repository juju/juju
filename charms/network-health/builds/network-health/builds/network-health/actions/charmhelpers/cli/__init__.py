# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import inspect
import argparse
import sys

from six.moves import zip

import charmhelpers.core.unitdata


class OutputFormatter(object):
    def __init__(self, outfile=sys.stdout):
        self.formats = (
            "raw",
            "json",
            "py",
            "yaml",
            "csv",
            "tab",
        )
        self.outfile = outfile

    def add_arguments(self, argument_parser):
        formatgroup = argument_parser.add_mutually_exclusive_group()
        choices = self.supported_formats
        formatgroup.add_argument("--format", metavar='FMT',
                                 help="Select output format for returned data, "
                                      "where FMT is one of: {}".format(choices),
                                 choices=choices, default='raw')
        for fmt in self.formats:
            fmtfunc = getattr(self, fmt)
            formatgroup.add_argument("-{}".format(fmt[0]),
                                     "--{}".format(fmt), action='store_const',
                                     const=fmt, dest='format',
                                     help=fmtfunc.__doc__)

    @property
    def supported_formats(self):
        return self.formats

    def raw(self, output):
        """Output data as raw string (default)"""
        if isinstance(output, (list, tuple)):
            output = '\n'.join(map(str, output))
        self.outfile.write(str(output))

    def py(self, output):
        """Output data as a nicely-formatted python data structure"""
        import pprint
        pprint.pprint(output, stream=self.outfile)

    def json(self, output):
        """Output data in JSON format"""
        import json
        json.dump(output, self.outfile)

    def yaml(self, output):
        """Output data in YAML format"""
        import yaml
        yaml.safe_dump(output, self.outfile)

    def csv(self, output):
        """Output data as excel-compatible CSV"""
        import csv
        csvwriter = csv.writer(self.outfile)
        csvwriter.writerows(output)

    def tab(self, output):
        """Output data in excel-compatible tab-delimited format"""
        import csv
        csvwriter = csv.writer(self.outfile, dialect=csv.excel_tab)
        csvwriter.writerows(output)

    def format_output(self, output, fmt='raw'):
        fmtfunc = getattr(self, fmt)
        fmtfunc(output)


class CommandLine(object):
    argument_parser = None
    subparsers = None
    formatter = None
    exit_code = 0

    def __init__(self):
        if not self.argument_parser:
            self.argument_parser = argparse.ArgumentParser(description='Perform common charm tasks')
        if not self.formatter:
            self.formatter = OutputFormatter()
            self.formatter.add_arguments(self.argument_parser)
        if not self.subparsers:
            self.subparsers = self.argument_parser.add_subparsers(help='Commands')

    def subcommand(self, command_name=None):
        """
        Decorate a function as a subcommand. Use its arguments as the
        command-line arguments"""
        def wrapper(decorated):
            cmd_name = command_name or decorated.__name__
            subparser = self.subparsers.add_parser(cmd_name,
                                                   description=decorated.__doc__)
            for args, kwargs in describe_arguments(decorated):
                subparser.add_argument(*args, **kwargs)
            subparser.set_defaults(func=decorated)
            return decorated
        return wrapper

    def test_command(self, decorated):
        """
        Subcommand is a boolean test function, so bool return values should be
        converted to a 0/1 exit code.
        """
        decorated._cli_test_command = True
        return decorated

    def no_output(self, decorated):
        """
        Subcommand is not expected to return a value, so don't print a spurious None.
        """
        decorated._cli_no_output = True
        return decorated

    def subcommand_builder(self, command_name, description=None):
        """
        Decorate a function that builds a subcommand. Builders should accept a
        single argument (the subparser instance) and return the function to be
        run as the command."""
        def wrapper(decorated):
            subparser = self.subparsers.add_parser(command_name)
            func = decorated(subparser)
            subparser.set_defaults(func=func)
            subparser.description = description or func.__doc__
        return wrapper

    def run(self):
        "Run cli, processing arguments and executing subcommands."
        arguments = self.argument_parser.parse_args()
        argspec = inspect.getargspec(arguments.func)
        vargs = []
        for arg in argspec.args:
            vargs.append(getattr(arguments, arg))
        if argspec.varargs:
            vargs.extend(getattr(arguments, argspec.varargs))
        output = arguments.func(*vargs)
        if getattr(arguments.func, '_cli_test_command', False):
            self.exit_code = 0 if output else 1
            output = ''
        if getattr(arguments.func, '_cli_no_output', False):
            output = ''
        self.formatter.format_output(output, arguments.format)
        if charmhelpers.core.unitdata._KV:
            charmhelpers.core.unitdata._KV.flush()


cmdline = CommandLine()


def describe_arguments(func):
    """
    Analyze a function's signature and return a data structure suitable for
    passing in as arguments to an argparse parser's add_argument() method."""

    argspec = inspect.getargspec(func)
    # we should probably raise an exception somewhere if func includes **kwargs
    if argspec.defaults:
        positional_args = argspec.args[:-len(argspec.defaults)]
        keyword_names = argspec.args[-len(argspec.defaults):]
        for arg, default in zip(keyword_names, argspec.defaults):
            yield ('--{}'.format(arg),), {'default': default}
    else:
        positional_args = argspec.args

    for arg in positional_args:
        yield (arg,), {}
    if argspec.varargs:
        yield (argspec.varargs,), {'nargs': '*'}
