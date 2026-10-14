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
        ) AS protocol_version
    FROM latest l
    LEFT JOIN current cu ON cu.host = l.host AND cu.n = 1
    LEFT JOIN cards c ON c.card_hash = cu.card_hash
    WHERE l.n = 1
)
