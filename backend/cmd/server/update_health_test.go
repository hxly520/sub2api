package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalHealthURL(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8080": "http://127.0.0.1:8080/health",
		":8080":        "http://127.0.0.1:8080/health",
		"[::]:9090":    "http://127.0.0.1:9090/health",
		"127.0.0.1:80": "http://127.0.0.1:80/health",
	}
	for address, expected := range tests {
		actual, err := localHealthURL(address)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}
