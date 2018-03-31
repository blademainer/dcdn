package dcdn

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"
)

func TestHash(t *testing.T) {
	//create test hash
	d := []byte("This is a very simple test input!")
	hval := sha256.Sum256(d)
	h := Hash{
		HashType: "sha256",
		Hash:     hval[:],
		Len:      uint32(len(d)),
	}
	//test hash formatting
	if h.String() != fmt.Sprintf("sha256:%x:%d", hval[:], len(d)) {
		t.Fatalf("Bad hash formatting: %q", h)
	}
	//test verifying a good hash
	v, err := h.Verifier()
	if err != nil {
		t.Fatalf("Failed to create sha256 verifier: %q\n", err.Error())
	}
	n, err := v.Write(d)
	if err != nil {
		t.Fatalf("Failed to write to verifier: %q\n", err.Error())
	}
	if n != len(d) {
		t.Fatalf("Incomplete write (%d/%d)\n", n, len(d))
	}
	err = v.Verify()
	if err != nil {
		t.Fatalf("Unexpected verify error: %q\n", err.Error())
	}
	//test unrecognized hash errors
	_, err = Hash{
		HashType: "badhashname",
		Hash:     []byte{1, 2, 3},
		Len:      65,
	}.Verifier()
	if err != ErrUnrecognizedHash {
		t.Fatalf("Expected unrecognized hash but got %q\n", err.Error())
	}
	//excess data write
	_, err = v.Write([]byte{1, 2, 3})
	if err != ErrTooLong {
		t.Fatalf("Expected too long error but got %q\n", err.Error())
	}
	err = v.Verify()
	if err != ErrTooLong {
		t.Fatalf("Expected too long error but got %q\n", err.Error())
	}
	//test sha512
	h512 := sha512.Sum512(d[:])
	v, err = Hash{
		HashType: "sha512",
		Hash:     h512[:],
		Len:      h.Len,
	}.Verifier()
	if err != nil {
		t.Fatalf("Failed to create sha512 verifier: %q\n", err.Error())
	}
	//test short write
	n, err = v.Write(d[:2])
	if err != nil {
		t.Fatalf("Failed to write to verifier: %q\n", err.Error())
	}
	if n != 2 {
		t.Fatalf("Incomplete write (%d/2)\n", n)
	}
	err = v.Verify()
	if err != ErrTooShort {
		t.Fatalf("Expected too short error but got %q\n", err.Error())
	}
	//finish sha512 test
	n, err = v.Write(d[2:])
	if err != nil {
		t.Fatalf("Failed to write to verifier: %q\n", err.Error())
	}
	if n != len(d)-2 {
		t.Fatalf("Incomplete write (%d/%d)\n", n, len(d)-2)
	}
	err = v.Verify()
	if err != nil {
		t.Fatalf("SHA512 test failed: %q\n", err.Error())
	}
	//test hash mismatch
	v, err = h.Verifier()
	if err != nil {
		t.Fatalf("Failed to create sha256 verifier: %q\n", err.Error())
	}
	n, err = v.Write(append(d[:len(d)-1], 1))
	if err != nil {
		t.Fatalf("Failed to write to verifier: %q\n", err.Error())
	}
	if n != len(d) {
		t.Fatalf("Incomplete write (%d/%d)\n", n, len(d))
	}
	err = v.Verify()
	if err != ErrMismatch {
		t.Fatalf("Unexpected hash mismatch error but got %q\n", err.Error())
	}
}

func quickHash(t *testing.T, dat []byte) Hash {
	h, err := GenHash("sha256", func(w io.Writer) (uint32, error) {
		w.Write(dat)
		return uint32(len(dat)), nil
	})
	if err != nil {
		t.Fatalf("Failed to generate sha256 hash: %q\n", err.Error())
	}
	if h == nil {
		t.Fatal("Hash is nil!\n")
	}
	return *h
}

func testErr(t *testing.T, v *Hash, err error, expect error) {
	if v != nil {
		t.Fatalf("Value is not nil (got %v)\n", v)
	}
	if err.Error() != expect.Error() {
		t.Fatalf("Expected error %q but got %q\n", expect.Error(), err.Error())
	}
}

func TestGenHash(t *testing.T) {
	dat := []byte("This is another great data sample!")
	h := quickHash(t, dat)
	//check for consistency\
	h2, err := ParseHash(h.String())
	if err != nil {
		t.Fatalf("Failed to parse generated hash: %q\n", err.Error())
	}
	if h2.String() != h.String() {
		t.Fatalf("Inconsistency: %q => %q", h.String(), h2.String())
	}
	//test gen a bad hash type
	ha, err := GenHash("bashash", func(w io.Writer) (uint32, error) {
		t.Fatal("This should not happen\n")
		return 0, nil
	})
	testErr(t, ha, err, ErrUnrecognizedHash)
	//test write handler
	terr := errors.New("bleh")
	ha, err = GenHash("sha256", func(w io.Writer) (uint32, error) {
		return 0, terr
	})
	testErr(t, ha, err, terr)
}

func TestParseHash(t *testing.T) {
	//try parsing a good hash
	goodhash := quickHash(t, []byte{1, 2, 3}).String()
	h, err := ParseHash(goodhash)
	if err != nil {
		t.Fatalf("Failed to parse good hash: %q\n", err.Error())
	}
	if h.String() != goodhash {
		t.Fatalf("Hash re-parse consistency error: %q => %q\n", goodhash, h.String())
	}
	//empty hash
	h, err = ParseHash("")
	testErr(t, h, err, ErrInvalid)
	//wrong number of fields
	h, err = ParseHash("x:y")
	testErr(t, h, err, ErrInvalid)
	h, err = ParseHash("w:x:y:z")
	testErr(t, h, err, ErrInvalid)
	//invalid hash function
	h, err = ParseHash("badhash:ff:65")
	testErr(t, h, err, ErrUnrecognizedHash)
	//invalid hex
	h, err = ParseHash("sha256:k:65")
	testErr(t, h, err, hex.InvalidByteError('k'))
	//invalid number
	h, err = ParseHash("sha256:ff:xyz")
	testErr(t, h, err, &strconv.NumError{
		Err:  strconv.ErrSyntax,
		Num:  "xyz",
		Func: "ParseUint",
	})
}
