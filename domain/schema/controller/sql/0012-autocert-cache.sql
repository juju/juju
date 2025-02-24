CREATE TABLE autocert_cache (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    data TEXT NOT NULL,
    encoding TEXT NOT NULL,
    CONSTRAINT fk_autocert_cache_encoding
    FOREIGN KEY (encoding)
    REFERENCES autocert_cache_encoding (id)
);

-- NOTE(nvinuesa): This table only populated with *one* hard-coded value
-- (x509) because golang's autocert cache doesn't provide encoding in it's
-- function signatures, and in juju we are only using x509 certs. The value
-- of this table is to correctly represent the domain and already have a
-- list of possible encodings when we update our code in the future.
CREATE TABLE autocert_cache_encoding (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL
);

INSERT INTO autocert_cache_encoding VALUES
(0, 'x509');    -- Only x509 certs encoding supported today.
