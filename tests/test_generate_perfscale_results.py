"""Tests for assess_perf_test_simple module."""

import logging
from mock import Mock, patch
import StringIO
from textwrap import dedent

import generate_perfscale_results as gpr
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client


class TestFindActualStart(TestCase):
    example_output = dedent("""\
                         value

    1468551204: -nan
    1468554270: -nan
    1468554273: -nan
    1468554270: -nan
    1468554273: -nan
    1468554276: 1.7516817067e+08
    1468554279: 1.7500023467e+08
    1468554282: 1.7661269333e+08
    1468554285: 1.7819374933e+08""")

    def test_doesnt_choke_on_non_timestamp_lines(self):
        try:
            gpr.find_actual_start(self.example_output)
        except Exception:
            self.fail('Unexpected exception raised.')

    def test_returns_actual_start_timestamp(self):
        self.assertEqual(
            gpr.find_actual_start(self.example_output),
            '1468554276')


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = gpr.parse_args(
            ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                gpr.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):
    pass
