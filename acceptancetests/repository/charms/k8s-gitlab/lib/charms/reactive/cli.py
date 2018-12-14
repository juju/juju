# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import os
import shlex

from charmhelpers.cli import cmdline
from charmhelpers.core import templating
from charms.reactive import helpers
from charms.reactive import bus


@cmdline.subcommand()
@cmdline.test_command
def hook(*hook_patterns):
    """
    Check if the current hook matches one of the patterns.
    """
    return helpers._hook(hook_patterns)


@cmdline.subcommand()
@cmdline.test_command
def when(*desired_flags):
    """
    Alias of when_all.
    """
    return helpers._when_all(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_all(*desired_flags):
    """
    Check if all of the desired_flags are active and have changed.
    """
    return helpers._when_all(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_any(*desired_flags):
    """
    Check if any of the desired_flags are active and have changed.
    """
    return helpers._when_any(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_not(*desired_flags):
    """
    Alias of when_none.
    """
    return helpers._when_none(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_none(*desired_flags):
    """
    Check if none of the desired_flags are active and have changed.
    """
    return helpers._when_none(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_not_all(*desired_flags):
    """
    Check if at least one of the desired_flags is not active.
    """
    return helpers._when_not_all(desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def when_file_changed(*filenames):
    """
    Check if files have changed since the last time they were checked.
    """
    return helpers.any_file_changed(filenames)


@cmdline.subcommand()
@cmdline.test_command
def only_once(handler_id):
    """
    Check if handler has already been run in the past.
    """
    return not helpers.was_invoked(handler_id)


@cmdline.subcommand()
@cmdline.no_output
def mark_invoked(*handler_ids):
    """
    Record that the handler has been invoked, for use with only_once.
    """
    for handler_id in handler_ids:
        helpers.mark_invoked(handler_id)


@cmdline.subcommand()  # noqa: C901
def test(*handlers):
    """
    Combined test function to apply one or more tests to multiple handlers.

    Each handler spec given should be a single argument but can contain shell
    quotes to group the parts, and should follow the form:

        'HANDLER_NAME HANDLER_ID [TEST_NAME TEST_ARGS]...'

    Each TEST_ARGS value can have further shell quoting.  For example:

        charms.reactive test 'foo foo_id when "foo.connected foo.available" when_not foo.disabled'
    """
    passed = []
    for handler_spec in handlers:
        handler_name, handler_id, tests = _parse_handler_spec(handler_spec)
        result = True
        states = set()
        for test_name, test_args in tests:
            if test_name == 'hook':
                result &= hook(*test_args)
            elif test_name == 'when':
                result &= when(*test_args)
                states.update(test_args)
            elif test_name == 'when_all':
                result &= when_all(*test_args)
                states.update(test_args)
            elif test_name == 'when_any':
                result &= when_any(*test_args)
                states.update(test_args)
            elif test_name == 'when_not':
                result &= when_not(*test_args)
                states.update(test_args)
            elif test_name == 'when_none':
                result &= when_none(*test_args)
                states.update(test_args)
            elif test_name == 'when_not_all':
                result &= when_not_all(*test_args)
                states.update(test_args)
            elif test_name == 'when_file_changed':
                result &= when_file_changed(*test_args)
            elif test_name == 'only_once':
                result &= only_once(handler_id)
            else:
                raise ValueError('Invalid test: %s' % test_name)
        if states:
            result &= bus.FlagWatch.watch(handler_id, states)
        if result:
            passed.append(handler_name)
    return ','.join(passed)


def _parse_handler_spec(handler_spec):
    parts = shlex.split(handler_spec)
    handler_name, handler_id = parts[:2]
    # one or more pairs of test_name + test_args
    # test_args can be further shell quoted
    tests = zip(parts[2::2], map(shlex.split, parts[3::2]))
    return handler_name, handler_id, tests


@cmdline.subcommand()
@cmdline.no_output
def render_template(source, target):
    """
    Render a Jinja2 template from $CHARM_DIR/templates using the current
    environment variables as the template context.
    """
    templating.render(source, target, os.environ)
