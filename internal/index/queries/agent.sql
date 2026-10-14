SELECT host, name, description, protocol_version, ok, http_status, error, at, card_json
FROM host_summaries
WHERE host = :host;
