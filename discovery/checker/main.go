package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
)

func main() {
	var h string
	flag.StringVar(&h, "http", ":8080", "http serving port")
	flag.Parse()
	log.Println("Initializing. . . ")
	http.HandleFunc("check", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			log.Printf("form parse error: %q\n", err.Error())
			return
		}
		turl := r.FormValue("targ")
		if turl != "" {
			http.Error(w, "missing target URL", http.StatusBadRequest)
			return
		}
		var resp struct {
			Valid  bool   `json:"valid"`
			ErrMsg string `json:"error"`
		}
		targurl, err := url.Parse(turl)
		if err != nil {
			resp.ErrMsg = err.Error()
			goto sendresp
		}
		//mutate URL to use /checkcdn
		targurl.Path = path.Join(targurl.Path, "checkcdn")
		//send request to /checkcdn
		{
			g, err := http.Get(targurl.String())
			if err != nil {
				resp.ErrMsg = err.Error()
				goto sendresp
			}
			defer g.Body.Close()
			if g.Header.Get("X-DCDN") != "cache" {
				resp.ErrMsg = fmt.Sprintf("not a DCDN cache: X-DCDN header is %q", g.Header.Get("X-DCDN"))
				goto sendresp
			}
			var stat struct {
				Status string `json:"status"`
			}
			err = json.NewDecoder(g.Body).Decode(&stat)
			if err != nil {
				resp.ErrMsg = err.Error()
				goto sendresp
			}
			resp.Valid = true
		}
	sendresp:
		json.NewEncoder(w).Encode(resp)
	})
	errch := make(chan error)
	go func() {
		errch <- http.ListenAndServe(h, nil)
	}()
	log.Printf("Started server on %q\n", h)
	log.Fatalf("Server crashed with %q\n", (<-errch).Error())
}
