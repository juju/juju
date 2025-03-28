(juju_cpu_profile)=
# `juju_cpu_profile`

The `juju_cpu_profile` introspection function provides a CPU profile of the 
current Juju agent process.  This is useful for debugging performance issues and 
it is primarily intended for use by Juju developers.
Under the hood this uses the [pprof](https://golang.org/pkg/net/http/pprof/) 
package.

This function is equivalent to `juju_heap_profile` but for CPU profiling.

## Usage

The `juju_cpu_profile` function will take samples during 30 seconds.
You can write the output of this function to a file:

```python
juju_cpu_profile > cpu.prof
```

Then you can use the pprof tool to analyze the profile.

