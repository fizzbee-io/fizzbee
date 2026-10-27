package lib

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
)

func TestWriteJSONString(t *testing.T) {
	for _, s := range []string{
		"", "name", "Broker#3", "a b", `quo"te`, `back\slash`, "new\nline", "tab\tctl\x01", "héllo ☃", "bad\xffutf8",
	} {
		b := strings.Builder{}
		WriteJSONString(&b, s)
		got := b.String()
		var back string
		assert.NoError(t, json.Unmarshal([]byte(got), &back), got)
		if strings.Contains(s, "\xff") {
			assert.Equal(t, "bad�utf8", back)
		} else {
			assert.Equal(t, s, back)
		}
	}
	// Plain identifiers keep the old unescaped form byte for byte.
	b := strings.Builder{}
	WriteJSONString(&b, "txn_log")
	assert.Equal(t, `"txn_log"`, b.String())
}

func TestStructMarshalJSONEscapesNames(t *testing.T) {
	s := FromStringDict(starlark.String("record"), starlark.StringDict{
		`we"ird`: starlark.MakeInt(1),
		"plain":  starlark.String("x"),
	})
	out, err := s.MarshalJSON()
	assert.NoError(t, err)
	var m map[string]any
	assert.NoError(t, json.Unmarshal(out, &m), string(out))
	assert.Equal(t, float64(1), m[`we"ird`])
	assert.Equal(t, "x", m["plain"])
}
