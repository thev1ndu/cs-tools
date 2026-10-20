-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
--
-- WSO2 LLC. licenses this file to you under the Apache License,
-- Version 2.0 (the "License"); you may not use this file except
-- in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.

-- A runbook gets a name and a list of alert names (runbook_alert); each existing entry becomes a runbook named after its alert, with that one alert.
ALTER TABLE runbook ADD COLUMN IF NOT EXISTS name TEXT;

CREATE TABLE IF NOT EXISTS runbook_alert (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    runbook_id  UUID        NOT NULL REFERENCES runbook (id) ON DELETE CASCADE,
    alert_name  TEXT        NOT NULL CHECK (btrim(alert_name) <> '' AND char_length(alert_name) <= 1000),
    created_on  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- An alert name belongs to one runbook, ignoring case and surrounding spaces.
CREATE UNIQUE INDEX IF NOT EXISTS runbook_alert_alert_name_key ON runbook_alert (lower(btrim(alert_name)));
CREATE INDEX IF NOT EXISTS runbook_alert_runbook_id_idx ON runbook_alert (runbook_id);

-- Moves the old one-alert-per-row values across; skipped once runbook.alert_name is gone, so a re-run is a no-op.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = 'runbook'::regclass AND attname = 'alert_name' AND NOT attisdropped) THEN
        UPDATE runbook SET name = btrim(alert_name) WHERE name IS NULL;
        INSERT INTO runbook_alert (runbook_id, alert_name) SELECT id, btrim(alert_name) FROM runbook ON CONFLICT DO NOTHING;
        ALTER TABLE runbook DROP COLUMN alert_name;
    END IF;
END $$;

ALTER TABLE runbook ALTER COLUMN name SET NOT NULL;
ALTER TABLE runbook DROP CONSTRAINT IF EXISTS runbook_name_check;
ALTER TABLE runbook ADD CONSTRAINT runbook_name_check CHECK (btrim(name) <> '' AND char_length(name) <= 1000);

-- One runbook per name, ignoring case and surrounding spaces.
CREATE UNIQUE INDEX IF NOT EXISTS runbook_name_key ON runbook (lower(btrim(name)));

COMMENT ON TABLE runbook IS
    'Runbook catalog: a named runbook link (NULL when not written yet) for the alert names in runbook_alert.';
COMMENT ON TABLE runbook_alert IS
    'Alert names of a runbook; an incident whose subject contains one shows that runbook.';
