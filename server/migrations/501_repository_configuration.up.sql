CREATE TABLE repository_configuration (
 scope text NOT NULL,
 subject text NOT NULL,
 config jsonb NOT NULL DEFAULT '{}',
 revision bigint NOT NULL DEFAULT 1
);
