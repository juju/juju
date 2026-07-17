-- An operation is an overview of an action or commands run on a remote
-- target by the user. It will be linked to N number of tasks, depending
-- on the number of entities it is run on.
-- An operation can be an action (meaning there will be an entry in 
-- operation_action) or not. If there is no entry, then the operation is an 
-- exec.
CREATE TABLE operation (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- operation_id is a sequence number, and the sequence is shared with 
    -- the operation_task.task_id sequence.
    operation_id TEXT NOT NULL,
    summary TEXT,
    enqueued_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    parallel BOOLEAN DEFAULT false,
    execution_group TEXT
);

CREATE UNIQUE INDEX idx_operation_id
ON operation (
    operation_id, uuid, summary, enqueued_at, started_at, completed_at,
    parallel, execution_group
);

CREATE INDEX idx_operation_completed_enqueued_uuid
ON operation (completed_at, enqueued_at, uuid);

CREATE INDEX idx_operation_details
ON operation (
    uuid, operation_id, summary, enqueued_at, started_at, completed_at,
    parallel, execution_group
);

-- operation_action is a join table to link an operation to its charm_action.
CREATE TABLE operation_action (
    operation_uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    charm_action_key TEXT NOT NULL,
    CONSTRAINT fk_operation_uuid
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid),
    CONSTRAINT fk_charm_action
    FOREIGN KEY (charm_uuid, charm_action_key)
    REFERENCES charm_action (charm_uuid, "key")
);

CREATE INDEX idx_operation_action_charm_action_key_operation_uuid
ON operation_action (charm_action_key, operation_uuid);

-- A operation_task is the individual representation of an operation on a specific
-- receiver. Either a machine or unit.
CREATE TABLE operation_task (
    uuid TEXT NOT NULL PRIMARY KEY,
    operation_uuid TEXT NOT NULL,
    -- task_id is a sequence number, and the sequence is shared with 
    -- the operation.operation_id sequence.
    task_id TEXT NOT NULL,
    enqueued_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    CONSTRAINT fk_operation
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid)
);

CREATE UNIQUE INDEX idx_task_id
ON operation_task (
    task_id, uuid, operation_uuid, enqueued_at, started_at, completed_at
);

CREATE INDEX idx_operation_task_operation_uuid_details
ON operation_task (
    operation_uuid, uuid, task_id, enqueued_at, started_at, completed_at
);

CREATE INDEX idx_operation_task_details
ON operation_task (
    uuid, operation_uuid, task_id, enqueued_at, started_at, completed_at
);

-- operation_unit_task is a join table to link a task with its unit receiver.
CREATE TABLE operation_unit_task (
    task_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, unit_uuid),
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE INDEX idx_operation_unit_task_unit_uuid_task_uuid
ON operation_unit_task (unit_uuid, task_uuid);

-- operation_machine_task is a join table to link a task with its machine receiver.
CREATE TABLE operation_machine_task (
    task_uuid TEXT NOT NULL,
    machine_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, machine_uuid),
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE INDEX idx_operation_machine_task_machine_uuid_task_uuid
ON operation_machine_task (machine_uuid, task_uuid);

-- operation_task_output is a join table to link a task with where
-- its output is stored.
CREATE TABLE operation_task_output (
    task_uuid TEXT NOT NULL PRIMARY KEY,
    store_path TEXT NOT NULL,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_store_path
    FOREIGN KEY (store_path)
    REFERENCES object_store_metadata_path (path)
);

CREATE INDEX idx_operation_task_output_store_path_task_uuid
ON operation_task_output (store_path, task_uuid);

-- operation_task_status is the status of the task.
CREATE TABLE operation_task_status (
    task_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_task_status
    FOREIGN KEY (status_id)
    REFERENCES operation_task_status_value (id)
);

-- operation_task_status_value holds the possible status values for a task.
CREATE TABLE operation_task_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_operation_task_status_value_status_id
ON operation_task_status_value (status, id);

INSERT INTO operation_task_status_value VALUES
(0, 'error'),
(1, 'running'),
(2, 'pending'),
(3, 'failed'),
(4, 'cancelled'),
(5, 'completed'),
(6, 'aborting'),
(7, 'aborted');

-- operation_task_log holds log messages of the task.
CREATE TABLE operation_task_log (
    task_uuid TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid)
);

CREATE INDEX idx_operation_task_log_id
ON operation_task_log (task_uuid, created_at);

CREATE INDEX idx_operation_task_log_details
ON operation_task_log (task_uuid, created_at, content);

CREATE INDEX idx_operation_action_details
ON operation_action (operation_uuid, charm_uuid, charm_action_key);

CREATE INDEX idx_operation_task_status_details
ON operation_task_status (task_uuid, status_id, message, updated_at);

-- operation_parameter holds the parameters passed to an operation.
-- In the case of an action, these are the user-passed parameters, where the 
-- keys should match the charm_action's parameters.
-- In the case of an exec, these will contain the "command" and "timeout" 
-- parameters.
CREATE TABLE operation_parameter (
    operation_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (operation_uuid, "key"),
    CONSTRAINT fk_operation_uuid
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid)
);

CREATE INDEX idx_operation_parameter_details
ON operation_parameter (operation_uuid, "key", value);
