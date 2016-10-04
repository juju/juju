"""Tests for perf_graphing support module."""

import perf_graphing as pg
from tests import TestCase


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


class TestMongoStatsData(TestCase):

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
