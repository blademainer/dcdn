package dcdn

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func hashFile(f *os.File, hashtype string) (*Hash, error) {
	return GenHash(hashtype, func(w io.Writer) (uint32, error) {
		n, err := io.Copy(w, f)
		if n > int64(^uint32(0)) {
			return 0, errors.New("oversized file")
		}
		return uint32(n), err
	})
}

type hcEnt struct {
	lck       sync.Mutex
	file      string    //path to file
	hash      *Hash     //hash value
	timestamp time.Time //file modification time (set on hash update)
	lastused  time.Time //last time used
}

func (e *hcEnt) shouldPrune() bool {
	e.lck.Lock()
	defer e.lck.Unlock()
	return time.Since(e.lastused) > (10 * time.Minute) //evict after 10 minutes of inactivity
}

func (e *hcEnt) getHash(basedir string, hashtype string) (*Hash, error) {
	e.lck.Lock()
	defer e.lck.Unlock()
	fpath := filepath.Join(basedir, e.file)
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer func() {
		e.lastused = time.Now()
	}()
	inf, err := f.Stat()
	if err != nil {
		return nil, err
	}
	mt := inf.ModTime()
	if mt != e.timestamp { //out of date - update hash
		h, err := hashFile(f, hashtype)
		if err != nil {
			return nil, err
		}
		e.hash = h
		e.timestamp = mt
	}
	return e.hash, nil
}

//HashCache is a cache for hash values of files
type HashCache struct {
	lck      sync.RWMutex
	wch      chan *hreq
	hashtype string //hash type to use
	dir      string //content directory
}

type hreq struct {
	lck  sync.Mutex
	name string
	he   *hcEnt
	err  error
}

//Get opens a file in the cache and also gets its hash and modification time
func (hc *HashCache) Get(path string) (*os.File, *Hash, time.Time, error) {
	//send request
	var req hreq
	req.name = path
	req.lck.Lock()
	hc.wch <- &req
	req.lck.Lock()
	he, err := req.he, req.err
	if err != nil {
		return nil, nil, time.Unix(0, 0), err
	}
	hc.lck.RLock()
	defer hc.lck.RUnlock()
	f, err := os.Open(filepath.Join(hc.dir, path))
	if err != nil {
		return nil, nil, time.Unix(0, 0), err
	}
	h, err := he.getHash(hc.dir, hc.hashtype)
	if err != nil {
		f.Close()
		return nil, nil, time.Unix(0, 0), err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, time.Unix(0, 0), err
	}
	return f, h, info.ModTime(), nil
}

func (hc *HashCache) server() {
	etbl := make(map[string]*hcEnt)
	prunetimer := time.NewTicker(time.Minute)
	defer prunetimer.Stop()
	ch := make(chan *hreq, 2)
	hc.wch = ch
	hc.lck.Unlock()
	for {
		select {
		case <-prunetimer.C: //prune cache
			for i, v := range etbl {
				if v.shouldPrune() {
					delete(etbl, i)
				}
			}
		case r, ok := <-ch: //incoming request
			if !ok { //channel closed, shutdown
				return
			}
			func() { //process request
				defer r.lck.Unlock()
				if etbl[r.name] == nil {
					hc.lck.RLock()
					defer hc.lck.RUnlock()
					_, err := os.Stat(filepath.Join(hc.dir, r.name))
					if err != nil {
						r.err = err
						return
					}
					he := new(hcEnt)
					he.lastused = time.Now()
					he.timestamp = time.Unix(0, 0) //set timestamp to epoch so it is invalid
					r.he = he
					etbl[r.name] = he
				} else {
					r.he = etbl[r.name]
				}
			}()
		}
	}
}

//Close closes a HashCache
func (hc *HashCache) Close() {
	close(hc.wch)
}

//SetHashType sets the hash type
func (hc *HashCache) SetHashType(hashtype string) error {
	if hashreg[hashtype] == nil {
		return ErrUnrecognizedHash
	}
	hc.lck.Lock()
	defer hc.lck.Unlock()
	hc.hashtype = hashtype
	return nil
}

//NewHashCache creates a new HashCache (default hash type currently sha256 but dont rely on that)
func NewHashCache(dir string) (*HashCache, error) {
	if _, err := os.Stat(dir); err != nil { //check that we can access the dir
		return nil, err
	}
	hc := new(HashCache)
	hc.hashtype = "sha256"
	hc.dir = dir
	hc.lck.Lock()
	hc.server()
	hc.lck.Lock() //wait for server startup
	hc.lck.Unlock()
	return hc, nil
}
