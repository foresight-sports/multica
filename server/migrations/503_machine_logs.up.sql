CREATE TABLE machine_logs (
 machine_key text NOT NULL,
 snapshot jsonb NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now()
);
