CREATE TABLE metrics (
    id TEXT NOT NULL,
    type VARCHAR(7) NOT NULL,
    gauge_value DOUBLE PRECISION,
    counter_value BIGINT,
    PRIMARY KEY (id, type),
    CONSTRAINT metrics_type_check CHECK (type IN ('gauge', 'counter')),
    CONSTRAINT metrics_value_check CHECK (
        (type = 'gauge' AND gauge_value IS NOT NULL AND counter_value IS NULL)
        OR
        (type = 'counter' AND counter_value IS NOT NULL AND gauge_value IS NULL)
    )
);
