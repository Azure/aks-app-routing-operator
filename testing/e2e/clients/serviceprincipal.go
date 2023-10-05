package clients

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

type ServicePrincipal struct {
	ID           string
	AppID        string
	Name         string
	ClientSecret string
}

const charBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

func RandStringAlphaNum(n int) (string, error) {
	r := rand.Reader
	b := make([]byte, n)
	for i := range b {
		iRandBig, err := rand.Int(r, big.NewInt(int64(len(charBytes))))
		iRand := int(iRandBig.Int64())
		if err != nil {
			return "", fmt.Errorf("generating random string: %w", err)
		}
		b[i] = charBytes[iRand]
	}
	return string(b), nil
}
