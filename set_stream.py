#!/usr/bin/env python

from argparse import ArgumentParser
import json

from generate_simplestreams import json_dump


def main():
    parser = ArgumentParser()
    parser.add_argument('stream')
    parser.add_argument('in_file')
    parser.add_argument('out_file')
    args = parser.parse_args()
    with open(args.in_file) as in_file:
        stanzas = json.load(in_file)
    for stanza in stanzas:
        stanza['content_id'] = 'com.ubuntu.juju:{}:tools'.format(args.stream)
    json_dump(stanzas, args.out_file)


if __name__ == '__main__':
    main()
