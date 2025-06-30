CREATE TABLE upgrade_state_type (
    id INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_upgrade_state_type_type
ON upgrade_state_type (type);

INSERT INTO upgrade_state_type VALUES
(0, 'created'),
(1, 'started'),
(2, 'db-completed'),
(3, 'steps-completed'),
(4, 'error');

CREATE TABLE upgrade_info (
    uuid TEXT NOT NULL PRIMARY KEY,
    previous_version TEXT NOT NULL,
    target_version TEXT NOT NULL,
    state_type_id INT NOT NULL,
    CONSTRAINT fk_upgrade_info_upgrade_state_type
    FOREIGN KEY (state_type_id)
    REFERENCES upgrade_state_type (id)
);

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist. This states, that multiple upgrades can exist if they're
-- not active, but only one active upgrade can exist
CREATE UNIQUE INDEX idx_singleton_active_upgrade ON upgrade_info ((1)) WHERE state_type_id < 3;

CREATE TABLE upgrade_info_controller_node (
    uuid TEXT NOT NULL PRIMARY KEY,
    controller_node_id TEXT NOT NULL,
    upgrade_info_uuid TEXT NOT NULL,
    node_upgrade_completed_at TIMESTAMP,
    CONSTRAINT fk_controller_node_id
    FOREIGN KEY (controller_node_id)
    REFERENCES controller_node (controller_id),
    CONSTRAINT fk_upgrade_info
    FOREIGN KEY (upgrade_info_uuid)
    REFERENCES upgrade_info (uuid)
);

CREATE UNIQUE INDEX idx_upgrade_info_controller_node
ON upgrade_info_controller_node (controller_node_id, upgrade_info_uuid);
