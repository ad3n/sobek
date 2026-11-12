//go:build (amd64 || arm64) && !purego

package sobek

import (
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/segmentio/asm/base64"
	"github.com/segmentio/asm/cpu"
	"github.com/segmentio/asm/cpu/x86"
)

const base64SIMDInputSize = 4096

var base64SIMDBuffers = sync.Pool{
	New: func() any {
		return new([base64SIMDInputSize / 4 * 3]byte)
	},
}

func base64DecodeSIMD(a asciiString, decodeMap *[256]byte, dst []byte) (read, written int) {
	limit := min((len(a)-1)/4, len(dst)/3) * 4
	if limit < 256 {
		return
	}

	if runtime.GOARCH == "amd64" && !cpu.X86.Has(x86.AVX2) {
		return
	}

	encoding := base64.RawStdEncoding
	switch decodeMap {
	case &base64DecodeMap:
	case &base64DecodeMapUrl:
		encoding = base64.RawURLEncoding
	default:
		return
	}

	buffer := base64SIMDBuffers.Get().(*[base64SIMDInputSize / 4 * 3]byte)
	defer base64SIMDBuffers.Put(buffer)

	src := unsafe.Slice(unsafe.StringData(string(a)), len(a))
	for limit-read >= 256 {
		size := min(base64SIMDInputSize, limit-read)
		if decodeMap == &base64DecodeMapUrl {
			block := string(a[read : read+size])
			if strings.IndexByte(block, '+') >= 0 || strings.IndexByte(block, '/') >= 0 {
				return
			}
		}

		n, err := encoding.Decode(buffer[:], src[read:read+size])
		if err != nil || n != size/4*3 {
			return
		}

		copy(dst[written:], buffer[:n])
		read += size
		written += n
	}

	return
}
