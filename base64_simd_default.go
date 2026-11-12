//go:build purego || (!amd64 && !arm64)

package sobek

func base64DecodeSIMD(a asciiString, decodeMap *[256]byte, dst []byte) (read, written int) {
	return
}
