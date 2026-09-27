package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestPolicy(t *testing.T) {
	for _, succeedAt := range []int{1, 3, 5} {
		attempts := 0
		var delays []time.Duration
		p := Policy{Wait: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			return nil
		}}
		err := p.Do(context.Background(), func() error {
			attempts++
			if attempts == succeedAt {
				return nil
			}
			return errors.New("temporary")
		}, func(error) bool { return true })
		want := succeedAt
		if want > 4 {
			want = 4
		}
		if attempts != want || (err != nil) != (succeedAt > 4) {
			t.Fatalf("attempts=%d, error=%v", attempts, err)
		}
		expected := []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}
		if len(delays) != want-1 {
			t.Fatal(delays)
		}
		for i, d := range delays {
			if d != expected[i] {
				t.Fatal(delays)
			}
		}
	}
}

func TestPermanentErrorAndCancellation(t *testing.T) {
	permanent := errors.New("permanent")
	calls := 0
	err := (Policy{}).Do(context.Background(), func() error { calls++; return permanent }, func(error) bool { return false })
	if err != permanent || calls != 1 {
		t.Fatal(err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err = (Policy{}).Do(ctx, func() error { cancel(); return permanent }, func(error) bool { return true })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPostgresClassification(t *testing.T) {
	for _, code := range []string{"08000", "08003", "08006", "08007", "08P01", "23505", "42601"} {
		err := fmt.Errorf("wrapped: %w", &pq.Error{Code: pq.ErrorCode(code)})
		if IsPostgresConnectionError(err) != (code[:2] == "08") {
			t.Fatal(code)
		}
	}
}
