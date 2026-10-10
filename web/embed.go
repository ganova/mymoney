// Package web menyematkan hasil build frontend ke dalam binary Go.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist adalah isi folder dist/ hasil `npm run build`.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
