-- The most common tags of the hosts with a current card. :prefix is a LIKE pattern ('' for all).
SELECT t.value AS tag, count(*) AS hosts
FROM host_summaries h, json_each(h.tags) t
WHERE h.card_json IS NOT NULL
  AND (:prefix = '' OR t.value LIKE :prefix ESCAPE '\')
GROUP BY t.value
ORDER BY hosts DESC, tag
LIMIT :limit;
