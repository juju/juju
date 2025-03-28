(controller)=
# Controller

> See also: {ref}`manage-controllers`

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the boostrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`).
<!--
 -- each of which includes
    - a {ref}`unit agent <unit-agent>`,
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code,
    - a {ref}`controller agent <controller-agent>` (running, among other things, the Juju API server), and
    - a copy of [`juju-db`](https://snapcraft.io/juju-db), Juju's internal database. <p>
-->
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in an internal MongoDB database.
