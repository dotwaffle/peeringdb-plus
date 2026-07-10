package web

import (
	"fmt"
	"net/http"
	"strconv"
)

// fragmentPageSize bounds each lazy-loaded relation fragment page. The
// fragment handlers previously ran unbounded .All queries — an IX with
// tens of thousands of participants materialized every row into one
// response. Pages beyond the first arrive via the "Load more" row that
// templates append while another page remains.
const fragmentPageSize = 100

// fragmentOffset reads the ?offset= pagination cursor for fragment
// requests. Absent or invalid values mean the first page.
func fragmentOffset(r *http.Request) int {
	off, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || off < 0 {
		return 0
	}
	return off
}

// fragmentMoreURL returns the load-more URL for the page after off, or
// "" when the current page is the last (templates render no button).
func fragmentMoreURL(r *http.Request, off int, hasMore bool) string {
	if !hasMore {
		return ""
	}
	return fmt.Sprintf("%s?offset=%d", r.URL.Path, off+fragmentPageSize)
}
