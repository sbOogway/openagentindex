-- Hosts that match :text, filtered by :status ('ok', 'failed' or 'any').
-- The last column is the number of all matches, before :limit and :offset.
SELECT host, name, description, protocol_version, ok, http_status, error, at, COUNT(*) OVER ()
FROM host_summaries
WHERE (:status = 'any' OR (:status = 'ok') = (ok = 1))
  AND (:text = ''
       OR host LIKE :text ESCAPE '\'
       OR name LIKE :text ESCAPE '\'
       OR description LIKE :text ESCAPE '\')
ORDER BY name = '', lower(name), host
LIMIT :limit OFFSET :offset;
