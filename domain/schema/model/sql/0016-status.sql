-- Status values for unit and application workloads.
CREATE TABLE workload_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO workload_status_value VALUES
(0, 'unset'),
(1, 'unknown'),
(2, 'maintenance'),
(3, 'waiting'),
(4, 'blocked'),
(5, 'active'),
(6, 'terminated'),
(7, 'error');
