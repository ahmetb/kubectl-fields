package managed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFieldsV1Key_FieldPrefix(t *testing.T) {
	prefix, content := ParseFieldsV1Key("f:metadata")
	assert.Equal(t, "f", prefix)
	assert.Equal(t, "metadata", content)
}

func TestParseFieldsV1Key_AssociativePrefix(t *testing.T) {
	prefix, content := ParseFieldsV1Key(`k:{"name":"nginx"}`)
	assert.Equal(t, "k", prefix)
	assert.Equal(t, `{"name":"nginx"}`, content)
}

func TestParseFieldsV1Key_ValuePrefix(t *testing.T) {
	prefix, content := ParseFieldsV1Key(`v:"example.com/foo"`)
	assert.Equal(t, "v", prefix)
	assert.Equal(t, `"example.com/foo"`, content)
}

func TestParseFieldsV1Key_DotMarker(t *testing.T) {
	prefix, content := ParseFieldsV1Key(".")
	assert.Equal(t, ".", prefix)
	assert.Equal(t, "", content)
}

func TestParseFieldsV1Key_Malformed(t *testing.T) {
	prefix, content := ParseFieldsV1Key("noprefix")
	assert.Equal(t, "", prefix)
	assert.Equal(t, "noprefix", content)
}

func TestParseAssociativeKey_SingleField(t *testing.T) {
	result, err := ParseAssociativeKey(`{"name":"nginx"}`)
	require.NoError(t, err)
	assert.Equal(t, "nginx", result["name"])
}

func TestParseAssociativeKey_MultiField(t *testing.T) {
	result, err := ParseAssociativeKey(`{"containerPort":80,"protocol":"TCP"}`)
	require.NoError(t, err)
	assert.Equal(t, float64(80), result["containerPort"])
	assert.Equal(t, "TCP", result["protocol"])
}

func TestParseAssociativeKey_Invalid(t *testing.T) {
	_, err := ParseAssociativeKey("not-json")
	assert.Error(t, err)
}
