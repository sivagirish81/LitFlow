package util

import "errors"

var (
	ErrNoExtractableText = errors.New("no extractable text found in PDF")

	ErrQuotaExhausted = errors.New("provider quota exhausted")
	ErrRateLimited    = errors.New("provider rate limited")
	ErrTransient      = errors.New("transient provider error")
	ErrPermanent      = errors.New("permanent provider error")
	ErrContextTooLong = errors.New("context too long")
)
