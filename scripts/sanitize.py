#!/usr/bin/env python3

# Copyright 2017 Canonical Ltd.
# Generate a Javascript file that can be used to sanitize a Mongo database so
# it can be shared.
from __future__ import print_function

import sys

# This lists the collections and fields in those collections that need to be sanitized
to_sanitize = [
 ('users', ['passwordhash']),
 ('units', ['passwordhash']),
 ('machines', ['passwordhash']),
 ('settings', ['settings']),
 ('controllers', ['settings', 'cert', 'privatekey', 'caprivatekey', 'sharedsecret', 'systemidentity']),
 ('actions', ['message', 'results']),
 ('cloudCredentials', ['attributes']),
]

def generateScript():
    lines = []
    for collection, attributes in to_sanitize:
        # First we generate the sanitization of the collection itself
        # (we don't use .format() cause {} is used all the time in Javascript
        lines.append('print("updating collection %s for %s")' % (collection, attributes))
        lines.append('print(db.%s.update({}, {"$set": {' % (collection,))
        for attribute in attributes:
            lines.append('    "{}": "REDACTED",'.format(attribute))
        # replace the last ',' with a '}'
        lines[-1] = lines[-1][:-1] + '}'
        lines.append('}, {"multi": 1}))')

        # Now update the TXN records for insert
        for attribute in attributes:
            lines.append('print("updating insert txns for %s %s")' % (collection, attribute))
            lines.append('print(db.txns.update({"o.c": "%s", "o.i.%s": {"$exists": 1}}, {"$set": {' % (collection, attribute))
            lines.append('    "o.$.i.%s": "REDACTED"}' % (attribute,))
            lines.append('}, {"multi": 1}))')
            lines.append('print("updating update txns for %s %s")' % (collection, attribute))
            # Our TXN entries have a '$set' with a literal $ in them.
            # Apparently Mongo is perfectly fine for you to create documents
            # and query documents with $, but won't let you "update" documents
            # with a $ in them. so we just unset those fields instead of setting them to 'REDACTED'.
            lines.append('print(db.txns.update({"o.c": "%s", "o.u.$set.%s": {"$exists": 1}}, {"$unset": {' % (collection, attribute))
            lines.append('    "o.$.u.$set.%s": "1"}' % (attribute,))
            lines.append('}, {"multi": 1}))')
        lines.append('')
    return lines


def main(args):
    import argparse
    p = argparse.ArgumentParser(description="""Generate a JavaScript file for Mongo that can sanitize a Juju database.
This updates both the actual collections as well as the transactions that
updated or created them. Fields that are sensitive are either converted to
REDACTED or removed. (we are unable to REDACT some of the update transactions,
because of limitations with $ characters.)
""", epilog="""
Example: ./sanitize.py > sanitize.js && mongo CONNECT_ARGS ./sanitize.js
""")
    opts = p.parse_args(args)

    lines = generateScript()
    content = '\n'.join(lines) + '\n'
    print(content)


if __name__ == "__main__":
    main(sys.argv[1:])
