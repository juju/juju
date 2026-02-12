/*
The view v_address is unused, so we can safely drop it.

However, it is a fun story to tell why we **need** to drop it there.

Not dropping this view leads to the following error in schema unit tests (all tests):

```sh
=== RUN   <TestModelSchemaSuite/TestApplyDDLIdempotent>
    /home/gfouillet/wd/github.com/gfouillet/juju/domain/schema/model_schema_test.go:57
    package_test.go:50:
            c.Assert(err, tc.ErrorIsNil)
        ... value errors.frameTracer = errors.frameTracer{error:(*errors.Err)(0xc0003fb720), pc:0x69bcaa}
          ("applying schema patches: failed to apply patch 155: error in view v_address: no such column: fa.hostname")
--- FAIL: TestModelSchemaSuite/TestApplyDDLIdempotent (0.06s)
```

This is due to the fact that the view v_address expects the fqdn_address table has a column hostname. That is not the
case. So the view is broken. And ALTER another table causes SQLITE to recompute every view. Boom.
 */
DROP VIEW v_address;

CREATE TABLE secret_content_new (
    revision_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    content TEXT NOT NULL,
    CONSTRAINT chk_empty_name
    CHECK (name != ''),
    CONSTRAINT pk_secret_content_revision_uuid_name
    PRIMARY KEY (revision_uuid, name),
    CONSTRAINT fk_secret_content_secret_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

INSERT INTO secret_content_new
SELECT
    revision_uuid,
    name,
    content
FROM secret_content;

DROP TABLE secret_content;
ALTER TABLE secret_content_new RENAME TO secret_content;
