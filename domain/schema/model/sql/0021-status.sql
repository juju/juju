-- Status values for machines
CREATE TABLE machine_status_values (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO machine_status_values VALUES
(0, 'error'),
(1, 'started'),
(2, 'pending'),
(3, 'stopped'),
(4, 'down');

-- Status values for machine cloud instances
CREATE TABLE instance_status_values (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO instance_status_values VALUES
(0, 'unknown'),
(1, 'allocating'),
(2, 'running'),
(3, 'provisioning error');
