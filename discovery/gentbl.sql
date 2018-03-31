/* Check that we have PostGIS */
CREATE EXTENSION IF NOT EXISTS postgis;
/* Create servers table if it does not exist */
CREATE TABLE IF NOT EXISTS servers(
    loc geography(POINT) NOT NULL,
    url text NOT NULL,
    https boolean NOT NULL,
    lastUpdate bigint NOT NULL
);
/* Create geographic index if it does not exist */
CREATE INDEX IF NOT EXISTS dcdngeo ON servers(loc);
