# retry

Package `retry` provides functionality for retrying functions that can fail.

## Features

- `Attempts` - max number of `retry.Func` calls
- `Linear backoff` - waits for a period of time between `retry.Func` calls
- `Exponential backoff` - wait period increases after each `retry.Func` call in `2^attempt` times
- `Jitter` (both linear and exponential) - randomizes backoff to eliminate collisions
- `3 ways to use`

## Usage

Retryable function `retry.Func` should return
- `(false, nil)` on successful call
- `(true, error)` on temporary error
- `(false, error)` on permanent error

For example, there is `retry.Func`:

   ```go
   var i int
   needsThreeCallsToSuccess := func () (repeat bool, err error) {
       defer func () { i++ }
       if i < 3 {
           repeat, err = true, fmt.Errorf("needs 3 attempts")
       }
       return
   }
   ```

We can use `retry` in 3 different ways:
    
1. Fluent interface

    ```go
    err := retry.Attempts(2).ExponentialJitterBackoff(time.Second, 0.25).Do(context.TODO(), needsThreeCallsToSuccess)
    // Output: no attempts left: needs 3 attempts

2. Functional options

   ```go
   err := retry.Do(context.TODO(), needsThreeCallsToSuccess, retry.WithAttempts(2), retry.WithBackoff(time.Second), 
        retry.WithExponential(), retry.WithJitter(0.25))
   // Output: no attempts left: needs 3 attempts

3. Constructor

   ```go
   config := retry.Config{
      Attempts:    2,
      Backoff:     time.Millisecond,
      Exponential: true,
      Jitter:      0.25,
   }
      
   err := retry.New(config).Do(context.TODO(), needsThreeCallsToSuccess)
   // Output: no attempts left: needs 3 attempts
