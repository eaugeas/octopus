package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	assert.Equal(t,
		"configuration key log.format does not have option xml implemented",
		ErrNotImplemented{Key: "log.format", Value: "xml"}.Error())

	assert.Equal(t,
		"configuration key needs to be set server.host",
		ErrKeyNotSet{Key: "server.host"}.Error())

	assert.Equal(t,
		"configuration key log.level set to invalid value loud. Accepted values are: debug, info.",
		ErrInvalidValue{Key: "log.level", InvalidValue: "loud", Values: []string{"debug", "info"}}.Error())

	assert.Equal(t,
		"failed to parse flags boom",
		ErrParseFlags{Cause: errors.New("boom")}.Error())
}
