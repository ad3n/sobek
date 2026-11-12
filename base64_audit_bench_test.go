package sobek

import (
	"encoding/base64"
	"strings"
	"testing"
)

func BenchmarkBase64DecodeWorkloads(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
		kind string
	}{
		{"tiny", 16, ""}, {"threshold", 192, ""}, {"above_threshold", 193, ""}, {"1KiB", 1024, ""}, {"4KiB", 4096, ""}, {"1MiB", 1 << 20, ""},
		{"url", 1 << 20, "url"}, {"invalid_start", 1 << 20, "invalid_start"}, {"invalid_end", 1 << 20, "invalid_end"}, {"whitespace_start", 1 << 20, "whitespace_start"}, {"mime", 1 << 20, "mime"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			encoding := base64.StdEncoding
			decodeMap := &base64DecodeMap
			if tc.kind == "url" {
				encoding = base64.URLEncoding
				decodeMap = &base64DecodeMapUrl
			}

			input := encoding.EncodeToString([]byte(strings.Repeat("\xff", tc.size)))
			switch tc.kind {
			case "invalid_start":
				input = "!" + input[1:]
			case "invalid_end":
				input = input[:len(input)-4] + "!AAA"
			case "whitespace_start":
				input = " " + input
			case "mime":
				var builder strings.Builder
				for start := 0; start < len(input); start += 76 {
					builder.WriteString(input[start:min(start+76, len(input))])
					builder.WriteString("\r\n")
				}

				input = builder.String()
			}

			a := asciiString(input)
			dst := make([]byte, tc.size)
			b.ReportAllocs()
			for b.Loop() {
				base64DecodeAscii(a, a, decodeMap, base64LastChunkHandlingLoose, dst)
			}
		})
	}
}
