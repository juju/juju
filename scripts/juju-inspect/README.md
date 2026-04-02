# juju-inspect

Juju inspect is intended to analyse engine reports from a Juju controller. You
can feed it multiple engine reports, and it will tell you various information
about the state of the controllers.

### Run

Running it is extremely simple:

```sh
make install
juju-inspect report_1 report_2 report_3
```

The output should look like:

```sh
Analysis of Engine Report:

Manifolds:
        machine-0 has 69 manifolds
        machine-1 has 69 manifolds
        machine-2 has 69 manifolds

Start Counts:
        machine-0 start-count: 516
          - max: "is-primary-controller-flag" with: 189
        machine-1 start-count: 421
          - max: "is-primary-controller-flag" with: 178
        machine-2 start-count: 498
          - max: "machine-action-runner" with: 274
```