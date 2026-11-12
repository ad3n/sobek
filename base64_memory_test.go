//go:build (linux || darwin) && (amd64 || arm64)

package sobek

import (
	"bytes"
	"encoding/base64"
	"syscall"
	"testing"
	"unsafe"

	asmbase64 "github.com/segmentio/asm/base64"
)

func base64GuardedPage(t *testing.T) []byte {
	t.Helper()
	pageSize := syscall.Getpagesize()
	mapping, err := syscall.Mmap(-1, 0, pageSize*3, syscall.PROT_NONE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := syscall.Munmap(mapping); err != nil {
			t.Error(err)
		}
	})

	page := mapping[pageSize : pageSize*2 : pageSize*2]
	if err := syscall.Mprotect(page, syscall.PROT_READ|syscall.PROT_WRITE); err != nil {
		t.Fatal(err)
	}

	return page
}

func TestBase64SIMDGuardPages(t *testing.T) {
	source := base64GuardedPage(t)
	target := base64GuardedPage(t)
	for i := range source {
		source[i] = 'A'
	}

	if err := syscall.Mprotect(source, syscall.PROT_READ); err != nil {
		t.Fatal(err)
	}

	for _, atEnd := range []bool{false, true} {
		for size := 0; size <= 4096; size++ {
			src := source[:size:size]
			if atEnd {
				src = source[len(source)-size:]
			}

			outputSize := size / 4 * 3
			dst := target[:outputSize:outputSize]
			if atEnd {
				dst = target[len(target)-outputSize:]
			}

			for _, url := range []bool{false, true} {
				encoding := asmbase64.RawStdEncoding
				decodeMap := &base64DecodeMap
				if url {
					encoding = asmbase64.RawURLEncoding
					decodeMap = &base64DecodeMapUrl
				}

				if size%4 == 0 {
					n, err := encoding.Decode(dst, src)
					if err != nil || n != outputSize {
						t.Fatalf("decode size=%d: %d %v", size, n, err)
					}
				}

				a := asciiString(unsafe.String(unsafe.SliceData(src), len(src)))
				base64DecodeAscii(a, a, decodeMap, base64LastChunkHandlingLoose, dst)
			}
		}

		for size := 0; size <= 3072; size++ {
			src := source[:size:size]
			if atEnd {
				src = source[len(source)-size:]
			}

			for _, encoding := range []*asmbase64.Encoding{asmbase64.StdEncoding, asmbase64.URLEncoding, asmbase64.RawStdEncoding, asmbase64.RawURLEncoding} {
				outputSize := encoding.EncodedLen(size)
				dst := target[:outputSize:outputSize]
				if atEnd {
					dst = target[len(target)-outputSize:]
				}

				encoding.Encode(dst, src)
				want := base64.StdEncoding.EncodeToString(src)
				if outputSize < len(want) {
					want = want[:outputSize]
				}

				if !bytes.Equal(dst, []byte(want)) {
					t.Fatalf("encode size=%d mismatch", size)
				}
			}
		}
	}
}
