import json
import os
from StringIO import StringIO
from unittest import TestCase

from mock import patch

from generate_simplestreams import (
    FileNamer,
    generate_index,
    Item,
    items2content_trees,
    write_streams,
    )
from utils import temp_dir


class TestItems2ContentTrees(TestCase):

    def test_items2content_trees_empty(self):
        result = items2content_trees([], {})
        self.assertEqual({}, result)

    def test_items2content_trees_one(self):
        result = items2content_trees([
            Item('cid', 'pname', 'vname', 'iname',
                 {'data': 'foo'}),
            ], {'extra-data': 'bar'})
        self.assertEqual(
            {'cid': {
                'content_id': 'cid',
                'extra-data': 'bar',
                'format': 'products:1.0',
                'products': {'pname': {'versions': {'vname': {
                    'items': {'iname': {'data': 'foo'}}
                    }}}}
                }}, result)

    def test_items2content_trees_two_items(self):
        result = items2content_trees([
            Item('cid', 'pname', 'vname', 'iname',
                 {'data': 'foo'}),
            Item('cid', 'pname', 'vname', 'iname2',
                 {'data': 'bar'}),
            ], {'extra-data': 'bar'})
        self.assertEqual(
            {'cid': {
                'content_id': 'cid',
                'extra-data': 'bar',
                'format': 'products:1.0',
                'products': {'pname': {'versions': {'vname': {
                    'items': {
                        'iname': {'data': 'foo'},
                        'iname2': {'data': 'bar'},
                        }
                    }}}}
                }}, result)

    def test_items2content_trees_two_products(self):
        result = items2content_trees([
            Item('cid', 'pname', 'vname', 'iname',
                 {'data': 'foo'}),
            Item('cid', 'pname2', 'vname', 'iname',
                 {'data': 'bar'}),
            ], {'extra-data': 'bar'})
        self.assertEqual(
            {'cid': {
                'content_id': 'cid',
                'extra-data': 'bar',
                'format': 'products:1.0',
                'products': {
                    'pname': {'versions': {'vname': {
                        'items': {'iname': {'data': 'foo'}},
                        }}},
                    'pname2': {'versions': {'vname': {
                        'items': {'iname': {'data': 'bar'}},
                        }}},
                    }
                }}, result)

    def test_items2content_trees_two_versions(self):
        result = items2content_trees([
            Item('cid', 'pname', 'vname', 'iname',
                 {'data': 'foo'}),
            Item('cid', 'pname', 'vname2', 'iname',
                 {'data': 'bar'}),
            ], {'extra-data': 'bar'})
        self.assertEqual(
            {'cid': {
                'content_id': 'cid',
                'extra-data': 'bar',
                'format': 'products:1.0',
                'products': {'pname': {'versions': {
                    'vname': {
                        'items': {'iname': {'data': 'foo'}},
                        },
                    'vname2': {
                        'items': {'iname': {'data': 'bar'}},
                        },
                    }}}
                }}, result)

    def test_items2content_trees_two_content_ids(self):
        result = items2content_trees([
            Item('cid', 'pname', 'vname', 'iname',
                 {'data': 'foo'}),
            Item('cid2', 'pname', 'vname', 'iname',
                 {'data': 'bar'}),
            ], {'extra-data': 'bar'})
        self.assertEqual(
            {
                'cid': {
                    'content_id': 'cid',
                    'extra-data': 'bar',
                    'format': 'products:1.0',
                    'products': {'pname': {'versions': {
                        'vname': {
                            'items': {'iname': {'data': 'foo'}},
                            },
                        }}}
                    },
                'cid2': {
                    'content_id': 'cid2',
                    'extra-data': 'bar',
                    'format': 'products:1.0',
                    'products': {'pname': {'versions': {
                        'vname': {
                            'items': {'iname': {'data': 'bar'}},
                            },
                        }}}
                    },
                }, result)


class TestFileNamer(TestCase):

    def test_get_index_path(self):
        self.assertEqual('streams/v1/index.json', FileNamer.get_index_path())

    def test_get_content_path(self):
        self.assertEqual(
            'streams/v1/foo:bar.json', FileNamer.get_content_path('foo:bar'))


class FakeNamer:

    @staticmethod
    def get_index_path():
        return 'foo.json'

    @staticmethod
    def get_content_path(content_id):
        return '{}.json'.format(content_id)


def load_json(out_dir, filename):
    with open(os.path.join(out_dir, filename)) as index:
        return json.load(index)


class TestGenerateIndex(TestCase):

    updated = 'January 1 1970'

    def test_no_content(self):
        index_json = generate_index({}, self.updated, FakeNamer)
        self.assertEqual({
            'format': 'index:1.0', 'index': {}, 'updated': self.updated},
            index_json)

    def test_two_content(self):
        index_json = generate_index({
            'bar': {'products': {'prodbar': {}}},
            'baz': {'products': {'prodbaz': {}}},
            }, self.updated, FakeNamer)
        self.assertEqual({
            'format': 'index:1.0', 'updated': self.updated, 'index': {
                'bar': {
                    'path': 'bar.json',
                    'products': ['prodbar'],
                    },
                'baz': {
                    'path': 'baz.json',
                    'products': ['prodbaz'],
                    }
                }
            }, index_json)


def load_stream_dir(stream_dir):
    contents = {}
    for filename in os.listdir(stream_dir):
        contents[filename] = load_json(stream_dir, filename)
    return contents


class TestWriteStreams(TestCase):

    updated = 'January 1 1970'

    def test_no_content(self):
        with temp_dir() as out_dir, patch('sys.stderr', StringIO()):
            filenames = write_streams(out_dir, {}, self.updated, FakeNamer)
            contents = load_stream_dir(out_dir)
        self.assertEqual(['foo.json'], contents.keys())
        self.assertEqual([os.path.join(out_dir, 'foo.json')], filenames)
        self.assertEqual(generate_index({}, self.updated, FakeNamer),
                         contents['foo.json'])

    def test_two_content(self):
        trees = {
            'bar': {'products': {'prodbar': {}}},
            'baz': {'products': {'prodbaz': {}}},
            }
        with temp_dir() as out_dir, patch('sys.stderr', StringIO()):
            filenames = write_streams(out_dir, trees, self.updated, FakeNamer)
            contents = load_stream_dir(out_dir)
        self.assertItemsEqual(['foo.json', 'bar.json', 'baz.json'],
                              contents.keys())
        self.assertItemsEqual([
            os.path.join(out_dir, 'foo.json'),
            os.path.join(out_dir, 'bar.json'),
            os.path.join(out_dir, 'baz.json'),
            ], filenames)
        self.assertEqual(generate_index(trees, self.updated, FakeNamer),
                         contents['foo.json'])
        self.assertEqual({'products': {'prodbar': {}}}, contents['bar.json'])
        self.assertEqual({'products': {'prodbaz': {}}}, contents['baz.json'])
