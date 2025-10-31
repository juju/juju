CREATE TABLE architecture (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_architecture_name
ON architecture (name);

INSERT INTO architecture VALUES
(0, 'amd64'),
(1, 'arm64'),
(2, 'ppc64el'),
(3, 's390x'),
(4, 'riscv64');
