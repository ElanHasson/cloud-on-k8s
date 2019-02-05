package helpers

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultRetryDelay = 3 * time.Second
	defaultTimeout    = 3 * time.Minute
)

// ExitOnErr exits with code 1 if the given error is not nil
func ExitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println("Exiting.")
		os.Exit(1)
	}
}

// Eventually runs the given function until success,
// with a default timeout
func Eventually(f func() error) func(*testing.T) {
	return func(t *testing.T) {
		fmt.Printf("Retries (%s timeout): ", defaultTimeout)
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, defaultTimeout, defaultRetryDelay)
		fmt.Println()
		require.NoError(t, err)
	}
}

// TestifyTestingTStub mocks testify's TestingT interface
// so that we can use assertions outside a testing context
type TestifyTestingTStub struct {
	err error
}

// Errorf sets the error for the TestingTStub
func (t *TestifyTestingTStub) Errorf(msg string, args ...interface{}) {
	t.err = fmt.Errorf(msg, args...)
}

// ElementsMatch checks that both given slices contain the same elements
func ElementsMatch(listA interface{}, listB interface{}) error {
	t := TestifyTestingTStub{}
	assert.ElementsMatch(&t, listA, listB)
	if t.err != nil {
		return t.err
	}
	return nil
}
