package lib

import (
	"strings"
	"unicode/utf8"
)

// WriteJSONString writes s to b as a JSON string literal, quotes included.
// Names that need no escaping (the common case: identifiers) are written
// as-is between quotes, so the output for them is byte-identical to the
// previous unescaped form and existing state hashes are unchanged.
func WriteJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' {
				i++
				continue
			}
			b.WriteString(s[start:i])
			switch c {
			case '"', '\\':
				b.WriteByte('\\')
				b.WriteByte(c)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteString(`\u00`)
				b.WriteByte(hexDigits[c>>4])
				b.WriteByte(hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(s[start:i])
			b.WriteString(`�`)
			i++
			start = i
			continue
		}
		i += size
	}
	b.WriteString(s[start:])
	b.WriteByte('"')
}

const hexDigits = "0123456789abcdef"
