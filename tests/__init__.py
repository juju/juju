"""Testing helpers and base classes for better isolation."""

from contextlib import contextmanager
import logging
import os
import StringIO
import subprocess
import unittest

from mock import patch

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

    def setUp(self):
        super(TestCase, self).setUp()

        def _must_not_Popen(*args, **kwargs):
            """Tests may patch Popen but should never call it."""
            self.fail("subprocess.Popen(*{!r}, **{!r}) called".format(
                args, kwargs))

        self.addCleanup(setattr, subprocess, "Popen", subprocess.Popen)
        subprocess.Popen = _must_not_Popen

        self.addCleanup(setattr, os, "environ", os.environ)
        os.environ = {}

        setup_test_logging(self, self.log_level)


class FakeHomeTestCase(TestCase):
    """FakeHomeTestCase creates an isolated home dir for Juju to use."""

    def setUp(self):
        super(FakeHomeTestCase, self).setUp()
        self.home_dir = use_context(self, utility.temp_dir())
        os.environ["HOME"] = self.home_dir
        os.environ["PATH"] = os.path.join(self.home_dir, ".local", "bin")
        os.mkdir(os.path.join(self.home_dir, ".juju"))


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
