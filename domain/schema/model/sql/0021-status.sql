-- Status values for machines.
CREATE TABLE machine_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO machine_status_value VALUES
(0, 'error'),
(1, 'started'),
(2, 'pending'),
(3, 'stopped'),
(4, 'down');

-- Status values for machine cloud instances.
CREATE TABLE instance_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO instance_status_value VALUES
(0, 'unknown'),
(1, 'allocating'),
(2, 'running'),
(3, 'provisioning error');

-- Status values for unit agents.
CREATE TABLE unit_agent_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO unit_agent_status_value VALUES
(0, 'allocating'),
(1, 'executing'),
(2, 'idle'),
(3, 'failed'),
(4, 'lost'),
(5, 'rebooting');

-- Status values for unit workloads.
CREATE TABLE unit_workload_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO unit_workload_status_value VALUES
(0, 'unset'),
(1, 'unknown'),
(2, 'maintenance'),
(3, 'waiting'),
(4, 'blocked'),
(5, 'active'),
(6, 'terminated');
