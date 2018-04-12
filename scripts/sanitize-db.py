#!/usr/bin/env python3

# Copyright 2017 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
# Generate a Javascript file that can be used to sanitize a Mongo database so
# it can be shared.
from __future__ import print_function

import sys

# This lists the collections and fields in those collections that need to be sanitized
to_sanitize = [
    ('users', ['passwordhash', 'passwordsalt']),
    ('units', ['passwordhash']),
    ('machines', ['passwordhash']),
    ('settings', ['settings']),
    ('controllers', ['settings', 'cert', 'privatekey', 'caprivatekey', 'sharedsecret', 'systemidentity']),
    ('actions', ['parameters', 'message', 'results']),
    ('cloudCredentials', ['attributes']),
    ('statuses', ['statusinfo']),
]

def generateScript():
    # Create an index on the transactions so that the updates go faster
    yield 'print(new Date().toLocaleString())'
    yield 'db.txns.createIndex({"o.c": 1})'
    for collection, attributes in to_sanitize:
        # First we generate the sanitization of the collection itself
        # (we don't use .format() because {} is used all the time in Javascript
        yield 'print(new Date().toLocaleString())'
        yield 'print("updating collection %s for %s")' % (collection, attributes)
        yield 'print(db.%s.update({}, {"$set": {' % (collection,)
        inner = ['    "%s": "REDACTED"' % (a,) for a in attributes]
        yield ',\n'.join(inner) + '}'
        yield '}, {"multi": 1}))'

        # Now update the TXN records for insert and update
        for attribute in attributes:
            yield 'print(new Date().toLocaleString())'
            yield 'print("updating insert txns for %s %s")' % (collection, attribute)
            yield 'print(db.txns.update({"o.c": "%s", "o.i.%s": {"$exists": 1}}, {"$set": {' % (collection, attribute)
            yield '    "o.$.i.%s": "REDACTED"}' % (attribute,)
            yield '}, {"multi": 1}))'
            yield 'print(new Date().toLocaleString())'
            yield 'print("updating update txns for %s %s")' % (collection, attribute)
            # Our TXN entries have a '$set' with a literal $ in them.
            # Apparently Mongo is perfectly fine for you to create documents
            # and query documents with $, but won't let you "update" documents
            # with a $ in them. so we just unset those fields instead of setting them to 'REDACTED'.
            yield 'print(db.txns.update({"o.c": "%s", "o.u.$set.%s": {"$exists": 1}}, {"$unset": {' % (collection, attribute)
            yield '    "o.$.u.$set.%s": "1"}' % (attribute,)
            yield '}, {"multi": 1}))'
        yield ''
    yield 'db.txns.dropIndex({"o.c": 1})'
    yield 'print(new Date().toLocaleString())'


def main(args):
    import argparse
    p = argparse.ArgumentParser(description="""\
Generate a JavaScript file for Mongo that can sanitize a Juju database.
This updates both the actual collections as well as the transactions that
updated or created them. Fields that are sensitive are either converted to
REDACTED or removed. (we are unable to REDACT some of the update transactions,
because of limitations with $ characters.)
""", epilog="""
Example: ./sanitize-db.py | mongo CONNECT_ARGS
""")
    opts = p.parse_args(args)

    for line in generateScript():
        print(line)


if __name__ == "__main__":
    main(sys.argv[1:])
