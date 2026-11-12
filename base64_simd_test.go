package sobek

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func checkBase64SIMD(t testing.TB, input []byte, capacity int, mode base64LastChunkHandling, url bool) {
	t.Helper()
	decodeMap := &base64DecodeMap
	if url {
		decodeMap = &base64DecodeMapUrl
	}

	a := asciiString(input)
	u := make(unicodeString, len(input)+1)
	u[0] = 0xfeff
	for i, c := range input {
		u[i+1] = uint16(c)
	}

	got := bytes.Repeat([]byte{0xa5}, capacity+32)
	want := bytes.Clone(got)
	read, written, err := base64DecodeAscii(a, a, decodeMap, mode, got[16:16+capacity:16+capacity])
	wantRead, wantWritten, wantErr := base64DecodeUnicode(u, u, decodeMap, mode, want[16:16+capacity:16+capacity])
	if read != wantRead || written != wantWritten || fmt.Sprint(err) != fmt.Sprint(wantErr) || !bytes.Equal(got, want) {
		t.Fatalf("len=%d capacity=%d mode=%d url=%t: got (%d, %d, %v), want (%d, %d, %v), buffers equal=%t", len(input), capacity, mode, url, read, written, err, wantRead, wantWritten, wantErr, bytes.Equal(got, want))
	}
}

func TestBase64SIMDBoundaries(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	for _, size := range []int{0, 1, 2, 3, 31, 32, 33, 191, 192, 193, 3071, 3072, 3073, 6145} {
		data := make([]byte, size)
		random.Read(data)
		for _, url := range []bool{false, true} {
			encoding := base64.StdEncoding
			if url {
				encoding = base64.URLEncoding
			}

			padded := encoding.EncodeToString(data)
			for _, input := range []string{padded, strings.TrimRight(padded, "="), padded + "\n", padded + "!"} {
				for _, capacity := range []int{0, 1, 2, 3, 191, 192, 193, 3071, 3072, 3073, size, size + 1} {
					for mode := base64LastChunkHandlingLoose; mode <= base64LastChunkHandlingStop; mode++ {
						checkBase64SIMD(t, []byte(input), capacity, mode, url)
					}
				}
			}
		}
	}
}

func TestBase64SIMDSpecialCharacters(t *testing.T) {
	for _, url := range []bool{false, true} {
		for _, pos := range []int{0, 1, 2, 3, 31, 32, 63, 255, 256, 4095, 4096, 4097, 8191} {
			input := []byte(strings.Repeat("abcd", 2049))
			for c := 0; c < 256; c++ {
				input[pos] = byte(c)
				for mode := base64LastChunkHandlingLoose; mode <= base64LastChunkHandlingStop; mode++ {
					checkBase64SIMD(t, input, 6200, mode, url)
				}
			}
		}
	}
}

func TestBase64SIMDParallel(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for j := 0; j < 16; j++ {
				input := bytes.Repeat([]byte{byte('A' + i)}, 8200)
				checkBase64SIMD(t, input, 6150, base64LastChunkHandlingStrict, false)
			}
		})
	}
}

func TestBase64SIMDEncoding(t *testing.T) {
	r := New()
	random := rand.New(rand.NewSource(2))
	for _, size := range []int{0, 1, 2, 3, 27, 28, 47, 48, 49, 1024, 65536} {
		data := make([]byte, size)
		random.Read(data)
		array := r.newTypedArrayWithData(data, r.getUint8Array(), r.newUint8ArrayObject, nil).val
		for _, url := range []bool{false, true} {
			for _, omitPadding := range []bool{false, true} {
				encoding := base64.StdEncoding
				alphabet := "base64"
				if url {
					encoding = base64.URLEncoding
					alphabet = "base64url"
				}

				if omitPadding {
					encoding = encoding.WithPadding(base64.NoPadding)
				}

				options := r.NewObject()
				if err := options.Set("alphabet", alphabet); err != nil {
					t.Fatal(err)
				}

				if err := options.Set("omitPadding", omitPadding); err != nil {
					t.Fatal(err)
				}

				got := r.uint8ArrayProto_toBase64(FunctionCall{This: array, Arguments: []Value{options}})
				if got.String() != encoding.EncodeToString(data) {
					t.Fatalf("size=%d url=%t omitPadding=%t: encoding mismatch", size, url, omitPadding)
				}
			}
		}
	}
}

func FuzzBase64SIMD(f *testing.F) {
	f.Add([]byte("AAAA"), uint16(3), byte(0), false)
	f.Add([]byte(strings.Repeat("a-b_", 65)), uint16(195), byte(1), true)
	f.Add([]byte{}, uint16(0), byte(2), false)

	for _, input := range []string{strings.Repeat("abcd", 2049), strings.Repeat("a-b_", 2049), strings.Repeat("ABCD", 1024) + " \t\r\nZA==", strings.Repeat("ABCD", 1024) + "Zh=="} {
		f.Add([]byte(input), uint16(6200), byte(0), false)
	}

	f.Fuzz(func(t *testing.T, input []byte, capacity uint16, mode byte, url bool) {
		if len(input) > 16384 {
			t.Skip()
		}

		checkBase64SIMD(t, input, int(capacity)%16385, base64LastChunkHandlingLoose+base64LastChunkHandling(mode%3), url)
	})
}

func TestBase64SIMDRandomized(t *testing.T) {
	random := rand.New(rand.NewSource(3))
	for i := 0; i < 4000; i++ {
		size := random.Intn(9000)
		data := make([]byte, size)
		random.Read(data)
		url := i%2 == 0
		encoding := base64.StdEncoding
		if url {
			encoding = base64.URLEncoding
		}

		input := []byte(encoding.EncodeToString(data))
		for j := 0; j < i%5 && len(input) > 0; j++ {
			input[random.Intn(len(input))] = byte(random.Intn(256))
		}

		capacity := random.Intn(size + 4)
		if i%3 == 0 {
			capacity = size + 3
		}

		checkBase64SIMD(t, input, capacity, base64LastChunkHandlingLoose+base64LastChunkHandling(i%3), url)
	}
}
