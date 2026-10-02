-- name: PutMachineLogs :exec
INSERT INTO machine_logs(machine_key,snapshot) VALUES (@machine_key,@snapshot)
ON CONFLICT(machine_key) DO UPDATE SET snapshot=EXCLUDED.snapshot, received_at=now();

-- name: GetMachineLogs :one
SELECT snapshot,received_at FROM machine_logs WHERE machine_key = sqlc.arg(machine_key);
