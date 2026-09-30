CREATE TABLE schema_version(version INTEGER NOT NULL);
INSERT INTO schema_version VALUES(1);
CREATE TABLE workspaces(root TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE tasks(id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE sessions(id TEXT PRIMARY KEY, data TEXT NOT NULL);
INSERT INTO sessions VALUES('s-old','{"ID":"s-old","State":"idle"}');
