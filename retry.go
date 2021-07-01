package retry

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

const DefaultJitter float64 = 0.1

// Func is a retryable function.
// Should return (false, nil) when success,
// (true, error) when error is temporary,
// (false, error) when error is permanent.
type Func func() (retry bool, err error)

// Do works same as Retry.Do
func Do(ctx context.Context, call Func, opts ...Option) error {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return New(cfg).Do(ctx, call)
}

// Option configures Retry
type Option func(*Config)

// WithAttempts sets the max number of Func calls
func WithAttempts(attempts int) Option {
	return func(cfg *Config) {
		cfg.Attempts = attempts
	}
}

// WithBackoff sets the delay after failed Func call
func WithBackoff(duration time.Duration) Option {
	return func(cfg *Config) {
		cfg.Backoff = duration
	}
}

// WithExponential makes backoff exponential,
// see Config.Exponential
func WithExponential() Option {
	return func(cfg *Config) {
		cfg.Exponential = true
	}
}

// WithJitter applies jitter to backoff,
// see Config.Backoff
func WithJitter(jitter float64) Option {
	return func(cfg *Config) {
		cfg.Jitter = jitter
	}
}

type Config struct {
	// Attempts is the max number of Func calls
	Attempts int
	// Backoff defines the delay after failed Func call
	Backoff time.Duration
	// Exponential makes Backoff exponential:
	// backoff multiplied 2 raised to the current attempt.
	Exponential bool
	// Jitter applies jitter to backoff, expected to be in range [0.0, 1.0).
	// If the passed value out of the range, DefaultJitter is used.
	Jitter float64
}

func New(cfg Config) Retry {
	r := Attempts(cfg.Attempts)
	if cfg.Exponential {
		return r.ExponentialJitterBackoff(cfg.Backoff, cfg.Jitter)
	}
	return r.JitterBackoff(cfg.Backoff, cfg.Jitter)
}

// Backoff defines the delay after failed Func call.
type Backoff func(attempt int) time.Duration

// Retry defines a policy of retrying Func calls.
type Retry struct {
	// attempts is the max number of Func calls.
	attempts int
	// backoff defines the delay after failed Func call.
	backoff Backoff
}

// Attempts initializes Retry with the max number of Func calls
func Attempts(attempts int) Retry {
	return Retry{attempts: attempts}
}

// Backoff defines linear backoff between Func calls.
func (r Retry) Backoff(duration time.Duration) Retry {
	return r.JitterBackoff(duration, 0)
}

// ExponentialBackoff defines exponential backoff between Func calls.
// The backoff is multiplied times 2 raised to the current attempt.
// For example, if duration is 100ms, then backoff equals 100ms after first attempt,
// 800ms after fourth, 1600ms after fifth.
func (r Retry) ExponentialBackoff(duration time.Duration) Retry {
	return r.ExponentialJitterBackoff(duration, 0)
}

// JitterBackoff defines linear backoff with jitter between Func calls.
// Duration expected to be positive, jitter expected to be in range [0.0, 1.0).
// If jitter is out of the range, DefaultJitter is used.
func (r Retry) JitterBackoff(duration time.Duration, jitter float64) Retry {
	if duration > 0 {
		r.backoff = withJitter(linearBackoff(duration), jitter)
	}
	return r
}

// ExponentialJitterBackoff defines exponential backoff with jitter between Func calls.
// Duration expected to be positive, jitter expected to be in range [0.0, 1.0).
// If jitter is out of the range, DefaultJitter is used.
// The backoff is multiplied times 2 raised to the current attempt.
// For example, if duration is 100ms, then backoff equals 100ms after first attempt,
// 800ms after fourth, 1600ms after fifth.
func (r Retry) ExponentialJitterBackoff(duration time.Duration, jitter float64) Retry {
	if duration > 0 {
		r.backoff = withJitter(exponentialBackoff(duration), jitter)
	}
	return r
}

// Do calls Func until:
// 1. Func returns (false, ...)
// 2. Func returns (true, ...) but attempts exceeded
// 3. context cancellation signal received
func (r Retry) Do(ctx context.Context, call Func) error {
	if r.backoff == nil || r.attempts < 2 {
		return r.do(ctx, call)
	}
	return r.doWithBackoff(ctx, call)
}

func (r Retry) do(ctx context.Context, call Func) error {
	var (
		err   error
		retry bool
	)

	for attempt := 0; attempt < r.attempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if retry, err = call(); !retry {
				return err
			}
		}
	}

	return noAttemptsLeft{reason: err}
}

func (r Retry) doWithBackoff(ctx context.Context, call Func) error {
	// lazy initialization and destruction of timer, as usually
	// Func returns a successful result at the first call
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	var (
		err   error
		retry bool
		last  = r.attempts - 1
	)
	for attempt := 0; attempt < r.attempts; attempt++ {
		if retry, err = call(); !retry {
			return err
		}

		// skip backoff after last attempt
		if attempt == last {
			break
		}

		duration := r.backoff(attempt)
		if timer == nil {
			timer = time.NewTimer(duration)
		} else {
			timer.Reset(duration)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	return noAttemptsLeft{reason: err}
}

// withJitter wraps Backoff with jitter
func withJitter(backoff Backoff, jitter float64) Backoff {
	if jitter < 0 || jitter >= 1 {
		jitter = DefaultJitter
	}

	if jitter == 0 {
		return backoff
	}

	return func(attempt int) time.Duration {
		duration := backoff(attempt)
		return jitterUp(duration, jitter)
	}
}

// jitterUp applies jitter for duration
func jitterUp(duration time.Duration, jitter float64) time.Duration {
	seedOnce.Do(func() {
		randomizer = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
	// multiplier is in the range (1-jitter, 1+jitter)
	multiplier := 1 + jitter*(randomizer.Float64()*2-1)
	return time.Duration(float64(duration) * multiplier)
}

var (
	// randomizer generates jitter value
	randomizer *rand.Rand
	// seedOnce initializes randomizer
	seedOnce sync.Once

	linearBackoff = func(duration time.Duration) Backoff {
		return func(_ int) time.Duration { return duration }
	}

	exponentialBackoff = func(duration time.Duration) Backoff {
		return func(attempt int) time.Duration { return duration << attempt }
	}
)

type noAttemptsLeft struct {
	reason error
}

func (e noAttemptsLeft) Error() string {
	if e.reason != nil {
		return fmt.Sprintf("no attempts left: %s", e.reason.Error())
	}
	return "no attempts left"
}

func (e noAttemptsLeft) Unwrap() error {
	return e.reason
}
