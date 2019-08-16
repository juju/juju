# Schemadocs

Schemadocs generates API documentation from the API facades schema json file.
The output is a self contained html file that you can use in your browser to
help navigate the types found with in juju.

### Generation

To generate the html file, you need to ensure you have downloaded juju and have
the files checked out correctly on your filesystem. Then run the following
command (assuming you have go installed) from the root of the juju project:

```
go run ./scripts/schemadocs/schemadocs.go ./apiserver/facades/schema.json > output.html
```

Open the resulting output.html in your browser and enjoy.
