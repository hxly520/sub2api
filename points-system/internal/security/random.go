package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
)

func RandomToken(bytes int) (string, error) {
	if bytes < 16 {
		return "", fmt.Errorf("random token length must be at least 16 bytes")
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func RandomSteppedInt64(minimum, maximum, step int64) (int64, error) {
	if minimum < 0 || maximum < minimum || step <= 0 || (maximum-minimum)%step != 0 {
		return 0, fmt.Errorf("invalid random reward range")
	}
	steps := (maximum-minimum)/step + 1
	if steps <= 0 {
		return 0, fmt.Errorf("random reward range is too large")
	}
	choice, err := rand.Int(rand.Reader, big.NewInt(steps))
	if err != nil {
		return 0, err
	}
	return minimum + choice.Int64()*step, nil
}
