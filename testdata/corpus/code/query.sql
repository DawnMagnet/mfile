CREATE TABLE samples (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO samples (id, name) VALUES (1, 'qa');

SELECT s.id, s.name
FROM samples AS s
WHERE s.id = 1
ORDER BY s.name;
