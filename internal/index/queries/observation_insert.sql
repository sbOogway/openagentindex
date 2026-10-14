INSERT OR IGNORE INTO observations
    (id, observer, host, kind, at, ok, card_hash, http_status, latency_ms, error)
VALUES
    (:id, :observer, :host, :kind, :at, :ok, :card_hash, :http_status, :latency_ms, :error);
