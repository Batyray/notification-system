package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RequiredFields(t *testing.T) {
	assert.Panics(t, func() {
		New(Options{})
	}, "should panic when service is empty")

	assert.Panics(t, func() {
		New(Options{Service: "api"})
	}, "should panic when environment is empty")

	assert.Panics(t, func() {
		New(Options{Service: "api", Environment: "dev"})
	}, "should panic when version is empty")
}

func TestNew_ValidOptions(t *testing.T) {
	assert.NotPanics(t, func() {
		New(Options{
			Service:     "api",
			Environment: "development",
			Version:     "0.1.0",
		})
	})
}

func TestLogger_DefaultFields(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{
		Service:     "api",
		Environment: "production",
		Version:     "1.0.0",
		Writer:      &buf,
	})

	l.Info("test message")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "test message", entry["msg"])
	assert.Equal(t, "api", entry["service"])
	assert.Equal(t, "production", entry["environment"])
	assert.Equal(t, "1.0.0", entry["version"])
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{
		Service:     "api",
		Environment: "development",
		Version:     "0.1.0",
		Writer:      &buf,
		Format:      FormatJSON,
	})

	child := l.With("correlation_id", "abc-123", "request_id", "req-456")
	child.Info("request handled")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "abc-123", entry["correlation_id"])
	assert.Equal(t, "req-456", entry["request_id"])
	assert.Equal(t, "api", entry["service"])
}

func TestLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{
		Service:     "api",
		Environment: "development",
		Version:     "0.1.0",
		Writer:      &buf,
		Format:      FormatText,
	})

	l.Info("hello")
	output := buf.String()
	assert.Contains(t, output, "hello")
	assert.Contains(t, output, "service=api")
}
