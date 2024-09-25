# Design

Index of all architecture and design decisions made in the project. These
decisions are made to ensure that the project is maintainable, scalable, and
secure.

All designs should be enforced and followed by all developers working on the
project. If you have any questions or concerns about a decision, please reach
out to the project lead.

## Index

### Active

 - [context.Context parameter in functions](./context-parameter-in-functions.md)
 - [catacomb.Catacomb vs tomb.Tomb](./catacomb-vs-tomb.md)
 - [catacomb.Catacomb vs worker.Runner](./catacomb-vs-runner.md)

### Outdated

These design decisions are outdated and should be updated to reflect the current
state of the project.

 - [API Design Specification](./api-design-specification.md)
 - [API Implementation Guide](./api-implementation-guide.md)

## Contributing

If you have a design decision that you would like to add to the index, please
create a new markdown file in the `doc/design` directory. The file should
contain the following sections:

 - Summary: A brief summary of the design decision.
 - Context: The context in which the design decision was made.
 - Decision: The decision that was made.
 - Alternatives: Any alternatives that were considered (optional).
