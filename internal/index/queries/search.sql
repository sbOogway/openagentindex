-- Hosts that match :text, filtered by :status ('ok', 'failed' or 'any'), :binding and
-- :version ('' for any) and :tags (a JSON array; the host must have all of them).
-- The last column is the number of all matches, before :limit and :offset.
SELECT host, name, description, protocol_version, ok, http_status, error, at, tags, bindings, COUNT(*) OVER ()
FROM host_summaries
WHERE (:status = 'any' OR (:status = 'ok') = (ok = 1))
  AND (:text = ''
       OR host LIKE :text ESCAPE '\'
       OR name LIKE :text ESCAPE '\'
       OR description LIKE :text ESCAPE '\')
  AND (:binding = '' OR EXISTS (SELECT 1 FROM json_each(bindings) WHERE value = :binding))
  AND (:version = '' OR protocol_version = :version OR protocol_version LIKE :version || '.%')
  AND NOT EXISTS (
      SELECT 1 FROM json_each(:tags) wanted
      WHERE NOT EXISTS (SELECT 1 FROM json_each(tags) have WHERE have.value = wanted.value)
  )
ORDER BY name = '', lower(name), host
LIMIT :limit OFFSET :offset;
