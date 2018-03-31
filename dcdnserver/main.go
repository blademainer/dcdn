//Quick command to serve a directory with DCDN support
package main

import (
	"flag"
	"log"
	"net/http"

	".."
)

func main() {
	var dir string
	var h string
	flag.StringVar(&dir, "dir", ".", "directory to serve")
	flag.StringVar(&h, "http", ":8080", "http address to serve on")
	flag.Parse()
	hc, err := dcdn.NewHashCache(dir)
	if err != nil {
		log.Fatalf("Failed to create hash cache: %q\n", err.Error())
	}
	err = http.ListenAndServe(h, dcdn.FileServer{HashCache: hc})
	log.Fatalf("Server terminated with error %q\n", err.Error())
}
