-- host_summaries: the newest card check of each host and the fields of its current card.
-- The current card is the newest valid card. Only observers with a weight more than 0 count.
-- queries.go puts this block in front of the queries that read host_summaries,
-- so it has no final semicolon.
WITH checks AS (
    SELECT o.id, o.host, o.ok, o.http_status, o.error, o.at, o.card_hash
    FROM observations o
    JOIN observers w ON w.observer = o.observer AND w.weight > 0
    WHERE o.kind = 'card'
),
latest AS (
    SELECT *, ROW_NUMBER() OVER (PARTITION BY host ORDER BY at DESC, id DESC) AS n
    FROM checks
),
current AS (
    SELECT host, card_hash, ROW_NUMBER() OVER (PARTITION BY host ORDER BY at DESC, id DESC) AS n
    FROM checks
    WHERE ok = 1 AND card_hash IS NOT NULL
),
host_summaries AS (
    SELECT
        l.host,
        l.ok,
        l.http_status,
        l.error,
        l.at,
        c.card_json,
        coalesce(json_extract(c.card_json, '$.name'), '') AS name,
        coalesce(json_extract(c.card_json, '$.description'), '') AS description,
        coalesce(
            json_extract(c.card_json, '$.supportedInterfaces[0].protocolVersion'),
            json_extract(c.card_json, '$.protocolVersion'),
            ''
        ) AS protocol_version,
        (
            SELECT json_group_array(tag) FROM (
                SELECT DISTINCT lower(trim(t.value)) AS tag
                FROM json_each(c.card_json, '$.skills') s, json_each(s.value, '$.tags') t
                WHERE t.type = 'text' AND trim(t.value) <> ''
                ORDER BY tag
            )
        ) AS tags,
        (
            -- v1.0 cards list supportedInterfaces. v0.3 cards have url, preferredTransport
            -- (default JSONRPC) and additionalInterfaces.
            SELECT json_group_array(binding) FROM (
                SELECT DISTINCT binding FROM (
                    SELECT i.value ->> 'protocolBinding' AS binding
                    FROM json_each(c.card_json, '$.supportedInterfaces') i
                    UNION ALL
                    SELECT coalesce(c.card_json ->> 'preferredTransport', 'JSONRPC')
                    WHERE c.card_json ->> 'url' IS NOT NULL
                    UNION ALL
                    SELECT a.value ->> 'transport'
                    FROM json_each(c.card_json, '$.additionalInterfaces') a
                )
                WHERE binding IN ('JSONRPC', 'HTTP+JSON', 'GRPC')
                ORDER BY binding
            )
        ) AS bindings
    FROM latest l
    LEFT JOIN current cu ON cu.host = l.host AND cu.n = 1
    LEFT JOIN cards c ON c.card_hash = cu.card_hash
    WHERE l.n = 1
)
