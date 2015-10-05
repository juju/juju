#!/usr/bin/env python3
from argparse import ArgumentParser
import json

from generate_simplestreams import json_dump


def parse_args():
    parser = ArgumentParser(description='Convert sstream-query output to JSON')
    parser.add_argument('input')
    parser.add_argument('output')
    return parser.parse_args()


def main():
    args = parse_args()
    output = []
    with open(args.input) as in_file:
        for line in in_file:
            output.append(eval(line))
    json_dump(output, args.output)


if __name__ == '__main__':
    main()
