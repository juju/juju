CREATE TABLE user (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    display_name TEXT,
    external BOOLEAN NOT NULL,
    removed BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_uuid TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_user_created_by_user
    FOREIGN KEY (created_by_uuid)
    REFERENCES user (uuid)
);
