CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL
);
CREATE TABLE tasks (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    done BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX tasks_owner_id_idx ON tasks(owner_id);
-- Public, deterministic demo identities. Never production credentials.
INSERT INTO users(id, name) VALUES (1, 'Alice'), (2, 'Bob');
