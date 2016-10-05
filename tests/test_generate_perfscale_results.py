"""Tests for assess_perf_test_simple module."""

from mock import patch
import StringIO
from textwrap import dedent

import generate_perfscale_results as gpr
from tests import (
    parse_error,
    TestCase,
)


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

    example_multivalue_output = dedent("""\
                             value1    value2

    1472708601: -nan -nan
    1472708604: -nan -nan
    1472708607: -nan -nan
    1472708610: -nan -nan
    1472708613: 7.5466666667e+02 5.8166666667e+02
    1472708616: 2.5555555556e+02 1.9833333333e+02
    1472708619: 1.3333333333e+01 1.1555555556e+01
    1472708622: 2.7444444444e+01 2.6222222222e+01""")

    def test_doesnt_choke_on_non_timestamp_lines(self):
        try:
            gpr.find_actual_start(self.example_output)
            gpr.find_actual_start(self.example_multivalue_output)
        except Exception:
            self.fail('Unexpected exception raised.')

    def test_returns_actual_start_timestamp(self):
        self.assertEqual(
            gpr.find_actual_start(self.example_output),
            '1468554276')

        self.assertEqual(
            gpr.find_actual_start(self.example_multivalue_output),
            '1472708613')
