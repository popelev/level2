-- Applied by collector EnsureSchema as well; kept for reference / manual migrate
CREATE SCHEMA IF NOT EXISTS collector;
CREATE TABLE IF NOT EXISTS collector.samples (
  time        TIMESTAMPTZ NOT NULL,
  tag_id      TEXT NOT NULL,
  value_num   DOUBLE PRECISION,
  value_text  TEXT,
  value_bool  BOOLEAN,
  quality     INT NOT NULL,
  PRIMARY KEY (time, tag_id)
);
