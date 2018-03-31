package main

import (
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	".."
	"github.com/elazarl/goproxy"
)

var cli = dcdn.NewClient()

func main() {
	var h string
	flag.StringVar(&h, "http", ":8080", "http bind")
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
	errchan := make(chan error)
	go func() {
		errchan <- http.ListenAndServe(h, proxy)
	}()
	log.Printf("Proxying on %q\n", h)
	log.Fatalf("http.ListenAndServe crashed: %q\n", (<-errchan).Error())
}
