package dcdn

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"
	"strings"
)

//registry of hash function
var hashreg = map[string](func() hash.Hash){}

//RegisterHash registers a hash function
func RegisterHash(name string, fn func() hash.Hash) {
	hashreg[name] = fn
}

//add common hash functions
func init() {
	RegisterHash("sha256", func() hash.Hash {
		return sha256.New()
	})
	RegisterHash("sha512", func() hash.Hash {
		return sha512.New()
	})
}

//Hash is a hash value used for DCDN
type Hash struct {
	HashType string
	Hash     []byte
	Len      uint32
}

//ErrInvalid is an error returned when the syntax of the hash is wrong
var ErrInvalid = errors.New("Invalid hash syntax")

//ParseHash parses a hash value
func ParseHash(str string) (*Hash, error) {
	parts := strings.Split(str, ":")
	if len(parts) != 3 {
		return nil, ErrInvalid
	}
	if hashreg[parts[0]] == nil {
		return nil, ErrUnrecognizedHash
	}
	htype := parts[0]
	hdat, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	l, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return nil, err
	}
	return &Hash{
		HashType: htype,
		Hash:     hdat,
		Len:      uint32(l),
	}, nil
}

//String serializes a hash (format type:valuehex:length)
func (h Hash) String() string {
	return fmt.Sprintf("%s:%x:%d", h.HashType, h.Hash, h.Len)
}

//ErrUnrecognizedHash is an error returned when there
var ErrUnrecognizedHash = errors.New("Unrecognized hash function")

//Verifier returns a new Verifier which can be used to verify the hash
func (h Hash) Verifier() (v *Verifier, err error) {
	defer func() {
		if err != nil {
			v = nil
		}
	}()
	hf := hashreg[h.HashType]
	if hf == nil {
		err = ErrUnrecognizedHash
		return
	}
	v = new(Verifier)
	v.h = hf()
	v.hd = h.Hash
	v.n = h.Len
	return
}

//ErrTooLong is an error returned when the input exceeds the associated length
var ErrTooLong = errors.New("Input too long")

//ErrTooShort is an error returned when the input is shorter than the associated length
var ErrTooShort = errors.New("Input too short")

//ErrMismatch is an error reported when the hash does not match
var ErrMismatch = errors.New("Hash mismatch")

//Verifier is a thing that verifies a hash
type Verifier struct {
	hd []byte
	h  hash.Hash
	n  uint32
}

func (v *Verifier) Write(dat []byte) (int, error) {
	if v.n < uint32(len(dat)) {
		v.n = 0
		v.h = nil
		return 0, ErrTooLong
	}
	n, err := v.h.Write(dat)
	v.n -= uint32(n)
	return n, err
}

//Verify checks a hash after writing
func (v *Verifier) Verify() error {
	switch {
	case v.h == nil:
		return ErrTooLong
	case v.n != 0:
		return ErrTooShort
	case !bytes.Equal(v.h.Sum(nil), v.hd):
		return ErrMismatch
	default:
		return nil
	}
}

//GenHash creates a new Hash
func GenHash(hashtype string, writehandler func(io.Writer) (uint32, error)) (*Hash, error) {
	hf := hashreg[hashtype]
	if hf == nil {
		return nil, ErrUnrecognizedHash
	}
	h := hf()
	n, err := writehandler(h)
	if err != nil {
		return nil, err
	}
	hdat := h.Sum(nil)
	return &Hash{
		HashType: hashtype,
		Hash:     hdat,
		Len:      n,
	}, nil
}
