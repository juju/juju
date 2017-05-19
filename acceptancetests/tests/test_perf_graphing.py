"""Tests for perf_graphing support module."""

from mock import mock_open, patch, Mock
import StringIO

import perf_graphing as pg
from tests import TestCase


class TestCreateMongodbRRDFiles(TestCase):

    def test_raises_SourceFileNotFound_for_missing_file(self):
        with self.assertRaises(pg.SourceFileNotFound):
            pg.create_mongodb_rrd_files('/foo', '/bar')

    def test_create_mongodb_rrd_files(self):
        results_dir = '/foo/test/'
        destination_dir = '/bar/test/'
        start_ts = '0000'
        end_ts = '9999'
        all_data = [Mock()]
        stat_data = (start_ts, end_ts, all_data)
        m_open = mock_open()
        with patch.object(pg, 'open', m_open):
            with patch.object(
                    pg, 'get_mongodb_stat_data',
                    autospec=True, return_value=stat_data) as m_gmst:
                with patch.object(
                        pg, '_create_mongodb_memory_database',
                        autospec=True) as m_mdb:
                    with patch.object(
                            pg, '_create_mongodb_query_database',
                            autospec=True) as m_qdb:
                        pg.create_mongodb_rrd_files(
                            results_dir, destination_dir)
        file_handle = m_open()
        m_gmst.assert_called_once_with(file_handle)
        m_mdb.assert_called_once_with(
            '/bar/test/mongodb_memory.rrd', start_ts, all_data)
        m_qdb.assert_called_once_with(
            '/bar/test/mongodb.rrd', start_ts, all_data)


class TestGetMongodbStatData(TestCase):

    def get_test_file_data(self):
        file_data = StringIO.StringIO()
        file_data.write(
            '    41   322     28     12       1    59|0     0.4    0.5       '
            '0  792M 103M   0|0   0|0  114k   199k   24 juju  PRI '
            '2016-10-05T22:54:23Z\n')
        file_data.write(
            '    11   231      9     *0       0     9|0     0.1    0.6       '
            '0  792M 105M   0|0   0|0 51.6k   131k   24 juju  '
            'PRI 2016-10-05T22:55:08Z\n')
        file_data.seek(0)
        return file_data

    def test_returns_populated_MongoStatsData_object(self):
        file_data = self.get_test_file_data()
        _, _, data = pg.get_mongodb_stat_data(file_data)

        self.assertEqual(data[0].timestamp, 1475708063)
        self.assertEqual(data[0].insert, 41)
        self.assertEqual(data[0].query, 322)
        self.assertEqual(data[0].update, 28)
        self.assertEqual(data[0].delete, 12)
        self.assertEqual(data[0].vsize, 792000000)
        self.assertEqual(data[0].res, 103000000)

    def test_returns_first_and_last_timestamps(self):
        file_data = self.get_test_file_data()

        first, last, _ = pg.get_mongodb_stat_data(file_data)
        self.assertEqual(first, 1475708063)
        self.assertEqual(last, 1475708108)

    def test_returns_multiple_results(self):
        file_data = self.get_test_file_data()
        first, last, data = pg.get_mongodb_stat_data(file_data)

        self.assertEqual(len(data), 2)
        self.assertEqual(data[0].timestamp, 1475708063)
        self.assertEqual(data[1].timestamp, 1475708108)


class TestValueToBytes(TestCase):

    def test_keeps_already_bytes(self):
        self.assertEqual(pg.value_to_bytes('0'), 0)
        self.assertEqual(pg.value_to_bytes('100'), 100)

    def test_converts_Kvalues_to_bytes(self):
        self.assertEqual(pg.value_to_bytes('1K'), 1000)
        self.assertEqual(pg.value_to_bytes('1.5K'), 1500)

    def test_converts_Mvalues_to_bytes(self):
        self.assertEqual(pg.value_to_bytes('1M'), 1000000)
        self.assertEqual(pg.value_to_bytes('2.5M'), 2500000)

    def test_converts_Gvalues_to_bytes(self):
        self.assertEqual(pg.value_to_bytes('1G'), 1000000000)
        self.assertEqual(pg.value_to_bytes('2.5G'), 2500000000)

    def test_raises_exception_on_unknown(self):
        with self.assertRaises(ValueError):
            pg.value_to_bytes('abc')

    def test_returns_inttype(self):
        self.assertIsInstance(pg.value_to_bytes('1M'), int)


class TestMongoStatsData(TestCase):

    def test_values_are_int_type(self):
        test_stats = pg.MongoStatsData(None, '*1', '*1', '*1', '*1', '1', '1')
        self.assertIsInstance(test_stats.insert, int)
        self.assertIsInstance(test_stats.query, int)
        self.assertIsInstance(test_stats.update, int)
        self.assertIsInstance(test_stats.delete, int)
        self.assertIsInstance(test_stats.vsize, int)
        self.assertIsInstance(test_stats.res, int)

    def test_converts_MandG_values_to_bytes(self):
        test_stats = pg.MongoStatsData(None, '0', '0', '0', '0', '1M', '1G')

        self.assertEqual(test_stats.vsize, 1000000)
        self.assertEqual(test_stats.res, 1000000000)

    def test_removes_star_indicators(self):
        test_stats = pg.MongoStatsData(None, '*1', '*1', '*1', '*1', '0', '0')
        self.assertEqual(test_stats.insert, 1)
        self.assertEqual(test_stats.query, 1)
        self.assertEqual(test_stats.update, 1)
        self.assertEqual(test_stats.delete, 1)

    def test_stores_timestamp(self):
        test_stats = pg.MongoStatsData(1234, '0', '0', '0', '0', '0', '0')
        self.assertEqual(test_stats.timestamp, 1234)
