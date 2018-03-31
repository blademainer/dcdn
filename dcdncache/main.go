package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	".."
)

var cli = dcdn.NewClient()

type cachent struct {
	sync.Mutex
	h        *dcdn.Hash
	fpath    string
	lastused time.Time
}

type cachereq struct {
	sync.Mutex
	h   *dcdn.Hash
	c   *cachent
	f   *os.File
	err error
}

type fiterator struct {
	n   uint64
	dir string
}

func (f *fiterator) next() (*os.File, error) {
	defer func() { f.n++ }()
	return os.OpenFile(filepath.Join(f.dir, fmt.Sprintf("%d.cache", f.n)), os.O_CREATE|os.O_WRONLY, 0700)
}

func main() {
	var dir string
	var h string
	flag.StringVar(&dir, "dir", "cache", "dir to use for caching")
	flag.StringVar(&h, "http", ":8080", "http to bind to")
	flag.Parse()
	delch := make(chan string, 20) //channel for files to be deleted
	for i := 0; i < 4; i++ {
		go func() { //worker that deletes files
			for fpath := range delch {
				err := os.Remove(fpath)
				if err != nil {
					log.Fatalf("Failed to delete file %q: %q\n", fpath, err.Error())
				}
			}
		}()
	}
	wch := make(chan *cachereq, 2)
	failch := make(chan *dcdn.Hash, 1)
	go func() { //cache manager
		ctbl := make(map[string]*cachent) //cache entry table
		var fit fiterator
		fit.dir = dir
		prunetimer := time.NewTicker(time.Minute)
		for {
			select {
			case <-prunetimer.C: //time to prune the cache
				for i, v := range ctbl {
					if func() bool {
						v.Lock()
						defer v.Unlock()
						if time.Since(v.lastused) > (5 * time.Minute) {
							select {
							case delch <- v.fpath: //delete it if possible
								v.fpath = ""
								delete(ctbl, i)
							default:
								return true //delete queue backed up - halt prune operation
							}
						}
						return false
					}() {
						log.Println("Delete queue overflow")
						continue
					}
				}
			case req := <-wch: //incoming request
				h := req.h
				hstr := h.String()
				ce := ctbl[hstr]
				if ce == nil {
					ce = new(cachent)
					ctbl[hstr] = ce
					f, err := fit.next()
					if err != nil {
						log.Fatalf("fiterator error: %q", err.Error())
					}
					ce.fpath = f.Name()
					req.f = f
					ce.Lock()
				}
				req.c = ce
				req.Unlock()
			case h := <-failch: //download failure notification
				delete(ctbl, h.String())
			}
		}
	}()
	http.HandleFunc("/cache", func(w http.ResponseWriter, r *http.Request) {
		//parse query
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			log.Printf("Failed to parse form: %q\n", err.Error())
			return
		}
		hstr, src := r.Header.Get("hash"), r.Header.Get("url")
		if hstr == "" {
			http.Error(w, "missing hash in query", http.StatusBadRequest)
			log.Println("missing hash in request query")
			return
		}
		if src == "" {
			http.Error(w, "missing url in query", http.StatusBadRequest)
			log.Println("missing url in request query")
			return
		}
		h, err := dcdn.ParseHash(hstr)
		if err != nil {
			http.Error(w, "invalid hash", http.StatusBadRequest)
			log.Printf("Failed to parse hash: %q\n", err.Error())
			return
		}
		srcu, err := url.Parse(src)
		if err != nil {
			http.Error(w, "invalid source url", http.StatusBadRequest)
			log.Printf("Failed to parse source url: %q\n", err.Error())
			return
		}
		//send cache request
		var req cachereq //build request
		req.Lock()
		req.h = h
		wch <- &req //send request
		req.Lock()  //wait for completion
		//load data
		if req.f != nil { //not in cache yet - load it
			err = func() error {
				resp, hash, err := cli.Get(srcu)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				veri, _ := hash.Verifier()
				_, err = io.Copy(req.f, io.TeeReader(resp.Body, veri))
				if err != nil {
					return err
				}
				err = veri.Verify()
				if err != nil {
					return err
				}
				return nil
			}()
			req.f.Close()
			if err != nil {
				http.Error(w, "failed to download data", http.StatusBadGateway)
				log.Printf("Failed to download data: %q\n", err.Error())
				fpath := req.c.fpath
				req.c.fpath = ""
				req.c.Unlock()
				select { //delete file
				case delch <- fpath:
					//NOTE: could cause deadlock if only done synchronously
				default:
					go func() { delch <- fpath }()
				}
				select { //notify of failure
				case failch <- h:
					//NOTE: could cause deadlock if only done synchronously
				default:
					go func() { failch <- h }()
				}
				return
			}
		} else {
			req.c.Lock()
		}
		defer req.c.Unlock()
		f, err := os.Open(req.c.fpath)
		if err != nil {
			http.Error(w, "failed to load cache file", http.StatusBadRequest)
			log.Printf("failed to open cache file: %q\n", err.Error())
			return
		}
		w.Header().Add("X-DCDN", "cache")
		w.Header().Set("X-DCDN-HASH", hstr)
		w.Header().Set("Etag", hstr)
		w.Header().Add("Cache-Control", "public")
		w.Header().Add("Cache-Control", "only-if-cached")
		w.Header().Add("Cache-Control", "immutable")
		w.Header().Add("Cache-Control", "no-transform")
		io.Copy(w, f)
	})
	http.HandleFunc("/checkcdn", func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add("X-DCDN", "cache")
		j := struct {
			Status string `json:"status"`
		}{
			Status: "active",
		}
		err := json.NewEncoder(w).Encode(j)
		if err != nil {
			http.Error(w, "Failed to encode status JSON", http.StatusInternalServerError)
			log.Printf("Failed to handle health check: %q\n", err.Error())
			return
		}
		log.Println("Responded to health check")
	})
	errch := make(chan error)
	go func() {
		errch <- http.ListenAndServe(h, nil)
	}()
	log.Printf("Server started on %q\n", h)
	log.Fatalf("http.ListenAndServe crashed: %q\n", (<-errch).Error())
}
