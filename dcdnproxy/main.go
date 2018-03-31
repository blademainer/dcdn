package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	".."
	"github.com/elazarl/goproxy"
)

var cli = dcdn.NewClient()

func main() {
	var h string
	var advert string
	var thisaddr string
	var shouldadvert bool
	flag.StringVar(&h, "http", ":8080", "http bind")
	flag.StringVar(&advert, "advert", "https://dcdn.projectpanux.com/register", "address to advertise at")
	flag.StringVar(&thisaddr, "this", "", "URL of local system")
	flag.BoolVar(&shouldadvert, "enableadvert", false, "whether the system should advertise")
	flag.Parse()
	proxy := goproxy.NewProxyHttpServer()
	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if r.Method == http.MethodGet {
			resp, h, err := cli.GetReq(r)
			if err != nil {
				return r, goproxy.NewResponse(r,
					goproxy.ContentTypeText, http.StatusBadGateway,
					"DCDN request failed")
			}
			v, _ := h.Verifier()
			buf := bytes.NewBuffer(nil)
			_, err = io.Copy(buf, io.TeeReader(resp.Body, v))
			if err != nil {
				return r, goproxy.NewResponse(r,
					goproxy.ContentTypeText, http.StatusBadGateway,
					"DCDN request failed")
			}
			err = v.Verify()
			if err != nil {
				return r, goproxy.NewResponse(r,
					goproxy.ContentTypeText, http.StatusBadGateway,
					"DCDN request hash mismatch")
			}
			resp.Body = ioutil.NopCloser(buf)
			return r, resp
		}
		return r, nil
	})
	herrch := make(chan error)
	go func() {
		herrch <- http.ListenAndServe(h, proxy)
	}()
	aerrch := make(chan error)
	if shouldadvert {
		//parse ad and this URLs
		advurl, err := url.Parse(advert)
		if err != nil {
			log.Fatalf("Failed to parse advert URL: %q\n", err.Error())
		}
		turl, err := url.Parse(thisaddr)
		if err != nil {
			log.Fatalf("Failed to parse server URL: %q\n", err.Error())
		}
		//generate advertisement request URL
		advurl.Query().Add("url", turl.String())
		go func() {
			tick := time.NewTicker(3 * time.Minute)
			defer tick.Stop()
			for _ = range tick.C {
				err := func() error {
					log.Println("Sending advertisement request. . . ")
					g, err := http.Get(advurl.String())
					if err != nil {
						return err
					}
					defer g.Body.Close()
					var resp struct {
						Ok     bool   `json:"ok"`
						ErrMsg string `json:"err"`
					}
					err = json.NewDecoder(g.Body).Decode(&resp)
					if err != nil {
						return err
					}
					if !resp.Ok {
						return fmt.Errorf("registration error: %q", resp.ErrMsg)
					}
					return nil
				}()
				if err != nil {
					aerrch <- err
					return
				}
			}
		}()
	}
	log.Printf("Proxying on %q\n", h)
	select {
	case err := <-herrch:
		log.Fatalf("Server terminated with error %q\n", err.Error())
	case err := <-aerrch:
		log.Fatalf("Advert failed with error %q\n", err.Error())
	}
}
