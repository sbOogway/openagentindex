package index

import (
	"embed"
	"fmt"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	querySearch       = query("search")
	queryAgent        = query("agent")
	queryObservations = query("observations")
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
