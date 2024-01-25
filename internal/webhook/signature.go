package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// verifySignature checks if the provided signature is valid for the given payload.
func VerifySignature(payload []byte, headerSignature string, secret string) bool {
	const signaturePrefix = "sha256="
	const signatureLength = 64 // Length of the hex representation of the sha256 hash
	sigLength := len(signaturePrefix) + signatureLength

	if secret == "" {
		log.Error().Msg("Empty webhook secret")
		return false
	}

	if len(headerSignature) == 0 {
		log.Error().Msgf("%s header has no value - did you forget to configure the webhook secret in github?", headerSignature)
		return false
	}

	if len(headerSignature) != sigLength {
		log.Error().Msgf("signature '%s' is not %d chars long - assuming the signature is bad", headerSignature, sigLength)
		return false
	}

	if !strings.HasPrefix(headerSignature, signaturePrefix) {
		log.Error().Msgf("signature has invalid format: %s", headerSignature)
		return false
	}

	signature := headerSignature[len(signaturePrefix):]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	sigIsValid := hmac.Equal([]byte(signature), []byte(expectedSignature))
	log.Debug().Msgf("signature [%s] verification result: %s", headerSignature, strconv.FormatBool(sigIsValid))
	return sigIsValid
}
