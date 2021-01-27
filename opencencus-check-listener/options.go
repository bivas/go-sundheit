package opencencus

type Option func(*CheckListener)

func WithClassification(classification string) Option {
	return func(listener *CheckListener) {
		listener.classification = classification
	}
}

// WithLivenessClassification sets the classification to "liveness"
func WithLivenessClassification() Option {
	return func(listener *CheckListener) {
		listener.classification = "liveness"
	}
}

// WithReadinessClassification sets the classification to "readiness"
func WithReadinessClassification() Option {
	return func(listener *CheckListener) {
		listener.classification = "readiness"
	}
}

// WithSetupClassification sets the classification to "setup"
func WithSetupClassification() Option {
	return func(listener *CheckListener) {
		listener.classification = "setup"
	}
}

func WithDefaults() Option {
	return func(listener *CheckListener) {
		for _, opt := range []Option{} {
			opt(listener)
		}
	}
}