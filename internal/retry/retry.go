package retry

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"time"
)

// Policy permits replacing the wait function in tests without changing delays.
// Its zero value uses cancellable timers.
type Policy struct {
	Wait func(context.Context, time.Duration) error
}

func (p Policy) Do(ctx context.Context, operation func() error, retriable func(error) bool) error {
	delays := [...]time.Duration{time.Second, 3 * time.Second, 5 * time.Second}
	wait := p.Wait
	if wait == nil {
		wait = waitContext
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil || attempt == len(delays) || !retriable(err) {
			return err
		}
		if err := wait(ctx, delays[attempt]); err != nil {
			return err
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// IsDialError excludes response/read errors: replaying an already delivered
// counter batch could count it twice.
func IsDialError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

func IsPostgresConnectionError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pgError interface{ SQLState() string }
	if errors.As(err, &pgError) {
		return strings.HasPrefix(pgError.SQLState(), "08")
	}
	var op *net.OpError
	return errors.Is(err, driver.ErrBadConn) || errors.As(err, &op)
}
