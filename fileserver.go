package dcdn

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

//FileServer is a DCDN-compatible HTTP file serving handler
type FileServer struct {
	HashCache *HashCache  //the underlying HashCache
	ErrLogger func(error) //function called to log errors (uses log lib if nil)
}

func (fs FileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, fmt.Sprintf("unsupported method %q", r.Method), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DCDN", "server")
	f, h, t, err := fs.HashCache.Get(r.URL.Path)
	if err != nil {
		if fs.ErrLogger != nil {
			fs.ErrLogger(err)
		} else {
			log.Printf("Failed to serve DCDN content: %q\n", err.Error())
		}
		if err != os.ErrNotExist {
			http.Error(w, "Failed to load hash", http.StatusInternalServerError)
		} else {
			http.Error(w, "404 not found", http.StatusNotFound)
		}
		return
	}
	defer f.Close()
	//caching stuff
	hstr := h.String()
	tstr := t.Format(http.TimeFormat)
	w.Header().Set("X-DCDN-HASH", hstr)
	w.Header().Set("Last-Modified", tstr)
	w.Header().Set("Etag", hstr)
	w.Header().Add("Cache-Control", "public")
	w.Header().Add("Cache-Control", "must-revalidate")
	w.Header().Add("Cache-Control", "proxy-revalidate")
	w.Header().Add("Cache-Control", "no-transform")
	//deal with browser cache hits
	if r.Header.Get("If-None-Match") == hstr || r.Header.Get("If-Modified-Since") == tstr || r.Header.Get("If-Unmodified-Since") == tstr {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	//send data
	io.Copy(w, f)
}
