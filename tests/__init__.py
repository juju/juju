"""Testing helpers and base classes for better isolation."""

from contextlib import contextmanager
import errno
import logging
import os
import StringIO
import subprocess
import unittest

from mock import patch
import yaml

import utility


@contextmanager
def stdout_guard():
    stdout = StringIO.StringIO()
    with patch('sys.stdout', stdout):
        yield
    if stdout.getvalue() != '':
        raise AssertionError(
            'Value written to stdout: {}'.format(stdout.getvalue()))


def use_context(test_case, context):
    result = context.__enter__()
    test_case.addCleanup(context.__exit__, None, None, None)
    return result


class TestCase(unittest.TestCase):
    """TestCase provides a better isolated version of unittest.TestCase."""

    log_level = logging.INFO
    test_environ = {}

    def setUp(self):
        super(TestCase, self).setUp()

        def _must_not_Popen(*args, **kwargs):
            """Tests may patch Popen but should never call it."""
            self.fail("subprocess.Popen(*{!r}, **{!r}) called".format(
                args, kwargs))

        self.addCleanup(setattr, subprocess, "Popen", subprocess.Popen)
        subprocess.Popen = _must_not_Popen

        self.addCleanup(setattr, os, "environ", os.environ)
        os.environ = dict(self.test_environ)

        setup_test_logging(self, self.log_level)

    def assertIsTrue(self, expr, msg=None):
        """Assert that expr is the True object."""
        self.assertIs(True, expr, msg)

    def assertIsFalse(self, expr, msg=None):
        """Assert that expr is the False object."""
        self.assertIs(False, expr, msg)


class FakeHomeTestCase(TestCase):
    """FakeHomeTestCase creates an isolated home dir for Juju to use."""

    def setUp(self):
        super(FakeHomeTestCase, self).setUp()
        self.home_dir = use_context(self, utility.temp_dir())
        os.environ['HOME'] = self.home_dir
        os.environ['PATH'] = os.path.join(self.home_dir, '.local', 'bin')
        os.mkdir(os.path.join(self.home_dir, '.juju'))
        self.set_public_clouds(get_default_public_clouds())

    def set_public_clouds(self, data_dict):
        """Set the data in the public-clouds.yaml file.

        :param data_dict: A dictionary of data, which is used to overwrite
            the data in public-clouds.yaml, or None, in which case the file
            is removed."""
        dest_file = os.path.join(self.home_dir, '.juju/public-clouds.yaml')
        if data_dict is None:
            try:
                os.remove(dest_file)
            except OSError as error:
                if error.errno != errno.ENOENT:
                    raise
        else:
            yaml.safe_dump(data_dict, dest_file)


def setup_test_logging(testcase, level=None):
    log = logging.getLogger()
    testcase.addCleanup(setattr, log, 'handlers', log.handlers)
    log.handlers = []
    testcase.log_stream = StringIO.StringIO()
    handler = logging.StreamHandler(testcase.log_stream)
    handler.setFormatter(logging.Formatter("%(levelname)s %(message)s"))
    log.addHandler(handler)
    if level is not None:
        testcase.addCleanup(log.setLevel, log.level)
        log.setLevel(level)


# suppress nosetests
setup_test_logging.__test__ = False


@contextmanager
def parse_error(test_case):
    stderr = StringIO.StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


@contextmanager
def temp_os_env(key, value):
    org_value = os.environ.get(key, '')
    os.environ[key] = value
    try:
        yield
    finally:
        os.environ[key] = org_value


def get_default_public_clouds():
    return {
        'clouds': {
            'foo': {
                'type': 'foo',
                'auth-types': ['access-key'],
                'regions': {
                    # This is the fake juju endpoint:
                    'bar': {'endpoint': 'bar.foo.example.com'},
                    'fee': {'endpoint': 'fee.foo.example.com'},
                    'fi': {'endpoint': 'fi.foo.example.com'},
                    'foe': {'endpoint': 'foe.foo.example.com'},
                    'fum': {'endpoint': 'fum.foo.example.com'},
                    }
                },
            'qux': {
                'type': 'fake',
                'auth-types': ['access-key'],
                'regions': {
                    'north': {'endpoint': 'north.qux.example.com'},
                    'south': {'endpoint': 'south.qux.example.com'},
                    }
                },
            }
        }
