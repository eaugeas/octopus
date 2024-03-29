package rpcs

import (
	"bytes"
	"testing"

	rw "github.com/eaugeas/octopus/readwrite"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestJsonDecoderDecode(t *testing.T) {
	buffer := bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n")
	m := make(map[string]string)

	err := JsonDecoder{}.Decode(buffer, &m)

	assert.Nil(t, err)
	assert.Equal(t, map[string]string{
		"potato":    "fried",
		"hamburger": "rare",
	}, m)
}

func TestJsonDecoderDecodeWithLimit(t *testing.T) {
	buffer := bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n")
	m := make(map[string]string)

	err := JsonDecoder{}.DecodeWithLimit(buffer, &m, rw.ReadLimitProps{
		FailOnExceed: true,
		Limit:        1024,
	})

	assert.Nil(t, err)
	assert.Equal(t, map[string]string{
		"potato":    "fried",
		"hamburger": "rare",
	}, m)
}

func TestJsonDecoderDecodeWithLimitTooSmall(t *testing.T) {
	buffer := bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n")
	m := make(map[string]string)

	err := JsonDecoder{}.DecodeWithLimit(buffer, &m, rw.ReadLimitProps{
		FailOnExceed: false,
		Limit:        10,
	})

	assert.Equal(t, "failed to decode json: unexpected EOF", err.Error())
}

func TestJsonDecoderDecodeWithLimitTooMuchData(t *testing.T) {
	buffer := bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n")
	m := make(map[string]string)

	err := JsonDecoder{}.DecodeWithLimit(buffer, &m, rw.ReadLimitProps{
		FailOnExceed: true,
		Limit:        10,
	})

	assert.True(t, errors.Is(err, rw.ErrLimitExceeded))
}
