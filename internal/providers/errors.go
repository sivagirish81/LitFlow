package providers

import "strings"

type ErrorType string

const (
	ErrorQuota     ErrorType = "quota"
	ErrorRate      ErrorType = "rate"
	ErrorTransient ErrorType = "transient"
	ErrorPermanent ErrorType = "permanent"
	ErrorContext   ErrorType = "context"
)

func ClassifyError(err error) ErrorType {
	if err == nil {
		return ""
	}
	e := strings.ToLower(err.Error())
	switch {
	case strings.Contains(e, "quota"), strings.Contains(e, "credit"), strings.Contains(e, "insufficient_quota"):
		return ErrorQuota
	case strings.Contains(e, "rate"), strings.Contains(e, "429"):
		return ErrorRate
	case strings.Contains(e, "context"), strings.Contains(e, "too long"):
		return ErrorContext
	case strings.Contains(e, "timeout"), strings.Contains(e, "temporarily"), strings.Contains(e, "unavailable"):
		return ErrorTransient
	default:
		return ErrorPermanent
	}
}
