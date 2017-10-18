package main

import (
	"fmt"
	"net/http"
)

// Hop-by-hop headers. These are removed when sent upstream.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                true,
	"Trailer":           true,
	"Transfer-Encoding": true,
	"Upgrade":           true,

	// We don't want to do range requests. Instead the server will return with
	// 200 and the full content, if we skip it. That's better so we can cache
	// the response.
	"Range": true,
}

type HTTPError int

func (e HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d %s", e, http.StatusText(e.Code()))
}
func (e HTTPError) Code() int {
	return int(e)
}
