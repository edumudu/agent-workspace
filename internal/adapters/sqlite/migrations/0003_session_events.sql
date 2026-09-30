CREATE TABLE session_events(id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, data TEXT NOT NULL);
CREATE INDEX session_events_by_session ON session_events(session_id, id);
