-- The newest observations of a host.
SELECT id, observer, kind, at, ok, http_status, latency_ms, error
FROM observations
WHERE host = :host
ORDER BY at DESC, id DESC
LIMIT :limit;
