from unittest import TestCase

from generate_simplestreams import (
    Item,
    items2content_trees,
    )


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
