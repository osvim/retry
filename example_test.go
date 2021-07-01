package retry_test

import (
	"context"
	"fmt"
	"time"

	"github.com/osvim/retry"
)

func ExampleDo() {
	var i int

	err := retry.Do(
		context.TODO(),
		func() (repeat bool, err error) {
			i++
			if i < 3 {
				return true, fmt.Errorf("needs 3 attempts")
			}
			return
		},
		retry.WithAttempts(2),
		retry.WithBackoff(time.Millisecond),
		retry.WithExponential(),
		retry.WithJitter(0.25),
	)

	fmt.Println(err)
	// Output: no attempts left: needs 3 attempts
}

func ExampleNew() {
	config := retry.Config{
		Attempts:    2,
		Backoff:     time.Millisecond,
		Exponential: true,
		Jitter:      0.25,
	}

	var i int

	err := retry.New(config).Do(context.TODO(), func() (repeat bool, err error) {
		i++
		if i < 3 {
			return true, fmt.Errorf("needs 3 attempts")
		}
		return
	})

	fmt.Println(err)
	// Output: no attempts left: needs 3 attempts
}

func ExampleRetry_Do() {
	var i int

	err := retry.Attempts(2).ExponentialJitterBackoff(time.Millisecond, 0.25).
		Do(context.TODO(), func() (repeat bool, err error) {
			i++
			if i < 3 {
				return true, fmt.Errorf("needs 3 attempts")
			}
			return
		})

	fmt.Println(err)
	// Output: no attempts left: needs 3 attempts
}
