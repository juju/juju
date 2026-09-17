#!/usr/bin/env python3

# Copyright 2017 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
# Generate a Javascript file that can be used to sanitize a Mongo database so
# it can be shared.
from __future__ import print_function

import sys

# This lists the collections and fields in those collections that need to be sanitized
to_sanitize = [
    ('users', ['secretkey', 'passwordhash', 'passwordsalt']),
    ('units', ['passwordhash']),
    ('machines', ['passwordhash']),
    ('applications', ['metric-credentials', 'passwordhash']),
    ('models', ['passwordhash', 'sla']),
    ('controllerNodes', ['password-hash']),
    ('settings', ['settings']),
    ('controllers', [
        'settings', 'cert', 'privatekey', 'caprivatekey', 'sharedsecret',
        'systemidentity', 'key', 'local-users-key', 'local-users-thirdparty-key',
        'external-users-thirdparty-key', 'offers-thirdparty-key',
    ]),
    ('actions', ['parameters', 'message', 'results']),
    ('cloudCredentials', ['attributes']),
    ('dockerResources', ['password']),
    ('sshrequests', ['password']),
    ('virtualhostkeys', ['hostkey']),
    ('autocertCache', ['data']),
    ('bakeryStorageItems', ['rootkey', 'item']),
    ('remoteEntities', ['token', 'macaroon']),
    ('remoteApplications', ['macaroon']),
    ('migrations', ['target-password', 'target-macaroons', 'target-token']),
    ('secretRevisions', ['data']),
    ('secretBackends', ['config']),
]
low_sensitivity_data_to_sanitize = [
    ('actions', ['messages']),
    ('statuses', ['statusinfo', 'statusdata']),
    ('statuseshistory', ['statusinfo', 'statusdata']),
]

def generateScript(sanitize_low_sensitivity_data=True):
    # Create an index on the transactions so that the updates go faster
    yield 'print(new Date().toLocaleString())'
    yield 'db.txns.createIndex({"o.c": 1})'
    collections = to_sanitize
    if sanitize_low_sensitivity_data:
        collections = to_sanitize + low_sensitivity_data_to_sanitize
    for collection, attributes in collections:
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
            if collection == 'actions' and attribute == 'messages':
                # Action logs are appended with $push instead of updated with $set.
                yield 'print(new Date().toLocaleString())'
                yield 'print("updating push txns for %s %s")' % (collection, attribute)
                yield 'print(db.txns.update({"o.c": "%s", "o.u.$push.%s": {"$exists": 1}}, {"$unset": {' % (collection, attribute)
                yield '    "o.$.u.$push.%s": "1"}' % (attribute,)
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
Example: ./sanitise-db.py | mongo CONNECT_ARGS
""")
    p.add_argument(
        "--keep-low-sensitivity-data",
        action="store_true",
        help="do not redact action logs or status data",
    )
    opts = p.parse_args(args)

    for line in generateScript(not opts.keep_low_sensitivity_data):
        print(line)


if __name__ == "__main__":
    main(sys.argv[1:])
