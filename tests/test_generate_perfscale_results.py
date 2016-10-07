"""Tests for assess_perf_test_simple module."""

from contextlib import contextmanager
from mock import patch, Mock
import StringIO
from textwrap import dedent

import generate_perfscale_results as gpr
import perf_graphing
from tests import (
    parse_error,
    TestCase,
)


class TestGenerateGraphImages(TestCase):

    @contextmanager
    def patch_generator(self, image):
        with patch.object(
                gpr, 'generate_graph_image',
                return_value=image, autospec=True) as m_ggi:
            yield m_ggi

    def test_generate_graph_image(self):
        image = Mock()
        base_dir = '/foo/test'
        results_dir = 'results'
        name = 'testing_name'
        generator = Mock()
        with patch.object(
                gpr, 'create_report_graph', return_value=image) as m_crg:
            self.assertEqual(
                image,
                gpr.generate_graph_image(
                    base_dir, results_dir, name, generator))
        m_crg.assert_called_once_with(
            '/foo/test/results', base_dir, name, generator)

    def test_generate_cpu_graph(self):
        image = Mock()
        with self.patch_generator(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_cpu_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'aggregation-cpu-average', 'cpu', perf_graphing.cpu_graph)

    def test_generate_memory_graph_calls_(self):
        image = Mock()
        with self.patch_generator(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'memory', 'memory', perf_graphing.memory_graph)

    def test_generate_network_graph(self):
        image = Mock()
        with self.patch_generator(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_network_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'interface-eth0', 'network', perf_graphing.network_graph)

    def test_generate_mongo_query_graph(self):
        image = Mock()
        with self.patch_generator(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_query_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'mongodb', 'mongodb', perf_graphing.mongodb_graph)

    def test_generate_mongo_memory_graph(self):
        image = Mock()
        with self.patch_generator(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo',
            'mongodb',
            'mongodb_memory',
            perf_graphing.mongodb_memory_graph)


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
