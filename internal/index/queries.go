package index

import (
	"embed"
	"fmt"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	querySearch            = query("host_summaries") + query("search") //nolint:gosec // G202: both parts are embedded files, not input.
	queryAgent             = query("host_summaries") + query("agent")  //nolint:gosec // G202: both parts are embedded files, not input.
	queryTags              = query("host_summaries") + query("tags")   //nolint:gosec // G202: both parts are embedded files, not input.
	queryObservations      = query("observations")
	queryNodeSelect        = query("node_select")
	queryNodeInsert        = query("node_insert")
	queryObserverInsert    = query("observer_insert")
	queryCardInsert        = query("card_insert")
	queryObservationInsert = query("observation_insert")
)

// query returns the SQL of queries/<name>.sql. The files are embedded at build time,
// so a missing file is a programming error.
func query(name string) string {
	b, err := queryFiles.ReadFile("queries/" + name + ".sql")
	if err != nil {
		panic(fmt.Sprintf("index: %v", err))
	}
	return string(b)
}
