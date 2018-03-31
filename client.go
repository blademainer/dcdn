package dcdn

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
)

//ServerSelector is an interface for a system that selects cache servers
type ServerSelector interface {
	SelectServers() []*url.URL //list of cache server URLs to use
	ReportFailure(*url.URL)    //called by a Client when the URL does not work
	Close()                    //Closes the ServerSelector (if supported)
}

//Client is a DCDN client
type Client struct {
	lck    sync.RWMutex
	ss     ServerSelector
	hcl    *http.Client
	closed bool
}

//SetSelector sets the server selector to use (default: no cache)
func (c *Client) SetSelector(ss ServerSelector) {
	if c.closed {
		panic(errors.New("Attempted to set a selector on a closed client"))
	}
	c.lck.Lock()
	defer c.lck.Unlock()
	c.ss = ss
}

//SetHTTPClient sets the HTTP client used (default: http.DefaultClient)
func (c *Client) SetHTTPClient(cli *http.Client) {
	if c.closed {
		panic(errors.New("Attempted to set a http client on a closed client"))
	}
	c.lck.Lock()
	defer c.lck.Unlock()
	c.hcl = cli
}

func (c *Client) getServers() ([]*url.URL, *http.Client) {
	c.lck.RLock()
	defer c.lck.RUnlock()
	var slst []*url.URL
	if c.ss != nil {
		slst = c.ss.SelectServers()
	}
	return slst, c.hcl
}

//Get is a wrapper around GetReq which uses a URL
func (c *Client) Get(u *url.URL) (o *http.Response, h *Hash, err error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	return c.GetReq(req)
}

//GetReq sends a request for content and returns an io.ReadCloser from which the content may be read
func (c *Client) GetReq(req *http.Request) (o *http.Response, h *Hash, err error) {
	if c.closed {
		err = errors.New("Client closed")
		return
	}
	//send request to server
	srvs, hcl := c.getServers()
	req.Header.Add("X-DCDN", "client")
	resp, err := hcl.Do(req)
	if err != nil {
		return
	}
	defer func() {
		if resp == o {
			resp.Body.Close()
		}
	}()
	if srvs != nil && resp.Header.Get("X-DCDN") == "server" {
		ha := resp.Header.Get("X-DCDN-HASH")
		h, err = ParseHash(ha) //load hash
		if err != nil {
			//use cache
			for _, s := range srvs {
				//build request URL
				su := new(url.URL)
				*su = *s
				su.Query().Add("hash", ha)
				su.Query().Add("url", req.URL.String())
				//send request
				var g *http.Response
				g, err = http.Get(su.String()) //attempt get request
				if err != nil || g.Header.Get("X-DCDN") != "cache" {
					//if it failed, or if the endpoint is not running DCDN, dont use this anymore
					func() {
						c.lck.RLock()
						defer c.lck.RUnlock()
						c.ss.ReportFailure(s)
					}()
					continue
				}
				//copy headers from origin server
				g.Header = resp.Header
				//set output and return
				o = g
				err = nil
				return
			}
		}
		//fallback to direct download
	}
	//load hash
	ha := resp.Header.Get("X-DCDN-HASH")
	h, _ = ParseHash(ha)
	//process request
	o = resp
	err = nil
	return
}

//NewClient creates a new Client (using http.DefaultClient as the http client)
func NewClient() *Client {
	cli := new(Client)
	cli.SetHTTPClient(http.DefaultClient)
	return cli
}

//Close closes the internal systems in the client
func (c *Client) Close() {
	c.lck.Lock()
	defer c.lck.Unlock()
	c.closed = true
	if c.ss != nil {
		c.ss.Close()
	}
}
