package main

import (
	"io/fs"

	"u/internal/web"
)

// staticFS returns a sub-filesystem rooted at "static/".
func staticFS() fs.FS {
	sub, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
