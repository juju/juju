CREATE TABLE machine_virtual_ssh_host_key (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    ssh_key TEXT NOT NULL,
    CONSTRAINT fk_machine_virtual_ssh_host_key_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE unit_virtual_ssh_host_key (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    ssh_key TEXT NOT NULL,
    CONSTRAINT fk_unit_virtual_ssh_host_key_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);
