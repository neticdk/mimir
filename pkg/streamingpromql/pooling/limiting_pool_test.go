// SPDX-License-Identifier: AGPL-3.0-only

package pooling

import (
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

const rejectedQueryName = "rejected_queries"

func TestLimitingPool_Unlimited(t *testing.T) {
	t.Run("[]promql.FPoint", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(0, metric)
		testUnlimitedPool(t, pool.GetFPointSlice, pool.PutFPointSlice, pool, FPointSize, reg)
	})

	t.Run("[]promql.HPoint", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(0, metric)
		testUnlimitedPool(t, pool.GetHPointSlice, pool.PutHPointSlice, pool, HPointSize, reg)
	})

	t.Run("promql.Vector", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(0, metric)
		testUnlimitedPool(t, pool.GetVector, pool.PutVector, pool, VectorSampleSize, reg)
	})

	t.Run("[]float64", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(0, metric)
		testUnlimitedPool(t, pool.GetFloatSlice, pool.PutFloatSlice, pool, Float64Size, reg)
	})

	t.Run("[]bool", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(0, metric)
		testUnlimitedPool(t, pool.GetBoolSlice, pool.PutBoolSlice, pool, BoolSize, reg)
	})
}

func testUnlimitedPool[E any, S ~[]E](t *testing.T, get func(int) (S, error), put func(S), pool *LimitingPool, elementSize uint64, reg *prometheus.Registry) {
	// Get a slice from the pool, the current and peak stats should be updated based on the capacity of the slice returned, not the size requested.
	s100, err := get(100)
	require.NoError(t, err)
	require.Equal(t, 128, cap(s100))
	require.Equal(t, 128*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 128*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Get another slice from the pool, the current and peak stats should be updated.
	s2, err := get(2)
	require.NoError(t, err)
	require.Equal(t, 2, cap(s2))
	require.Equal(t, 130*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 130*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Put a slice back into the pool, the current stat should be updated but peak should be unchanged.
	put(s100)
	require.Equal(t, 2*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 130*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Get another slice from the pool that doesn't take us over the previous peak.
	s5, err := get(5)
	require.NoError(t, err)
	require.Equal(t, 8, cap(s5))
	require.Equal(t, 10*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 130*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Get another slice from the pool that does take us over the previous peak.
	s200, err := get(200)
	require.NoError(t, err)
	require.Equal(t, 256, cap(s200))
	require.Equal(t, 266*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 266*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Ensure we handle nil slices safely.
	put(nil)
	require.Equal(t, 266*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 266*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	assertRejectedQueryCount(t, reg, 0)
}

func TestLimitingPool_Limited(t *testing.T) {
	t.Run("[]promql.FPoint", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(11*FPointSize, metric)
		testLimitedPool(t, pool.GetFPointSlice, pool.PutFPointSlice, pool, FPointSize, reg)
	})

	t.Run("[]promql.HPoint", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(11*HPointSize, metric)
		testLimitedPool(t, pool.GetHPointSlice, pool.PutHPointSlice, pool, HPointSize, reg)
	})

	t.Run("promql.Vector", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(11*VectorSampleSize, metric)
		testLimitedPool(t, pool.GetVector, pool.PutVector, pool, VectorSampleSize, reg)
	})

	t.Run("[]float64", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(11*Float64Size, metric)
		testLimitedPool(t, pool.GetFloatSlice, pool.PutFloatSlice, pool, Float64Size, reg)
	})

	t.Run("[]bool", func(t *testing.T) {
		reg, metric := createRejectedMetric()
		pool := NewLimitingPool(11*BoolSize, metric)
		testLimitedPool(t, pool.GetBoolSlice, pool.PutBoolSlice, pool, BoolSize, reg)
	})
}

func testLimitedPool[E any, S ~[]E](t *testing.T, get func(int) (S, error), put func(S), pool *LimitingPool, elementSize uint64, reg *prometheus.Registry) {
	// This method assumes that pool has been created with a limit set to 11x elementSize

	// Get a slice from the pool beneath the limit.
	s7, err := get(7)
	require.NoError(t, err)
	require.Equal(t, 8, cap(s7))
	require.Equal(t, 8*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 8*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	assertRejectedQueryCount(t, reg, 0)

	// Get another slice from the pool beneath the limit.
	s1, err := get(1)
	require.NoError(t, err)
	require.Equal(t, 1, cap(s1))
	require.Equal(t, 9*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 9*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	assertRejectedQueryCount(t, reg, 0)

	// Return a slice to the pool.
	put(s1)
	require.Equal(t, 8*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 9*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	assertRejectedQueryCount(t, reg, 0)

	// Try to get a slice where the requested size would push us over the limit.
	_, err = get(4)
	expectedError := fmt.Sprintf("the query exceeded the maximum allowed estimated amount of memory consumed by a single query (limit: %d bytes) (err-mimir-max-estimated-memory-consumption-per-query)", pool.MaxEstimatedMemoryConsumptionBytes)
	require.ErrorContains(t, err, expectedError)
	require.Equal(t, 8*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 9*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	assertRejectedQueryCount(t, reg, 1)

	// Try to get a slice where the requested size is under the limit, but the capacity of the slice returned by the pool is over the limit.
	// (We expect the pool to be configured with a factor of 2, so a slice of size 3 will be rounded up to 4 elements.)
	_, err = get(3)
	require.ErrorContains(t, err, expectedError)
	require.Equal(t, 8*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 9*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)

	// Make sure we don't increment the rejection count a second time for the same query.
	assertRejectedQueryCount(t, reg, 1)

	// Keep getting more slices from the pool up to the limit of 11 to make sure the failed allocations weren't counted.
	for i := 0; i < 3; i++ {
		s1, err = get(1)
		require.NoError(t, err)
		require.Equal(t, 1, cap(s1))
		require.Equal(t, uint64(9+i)*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
		require.Equal(t, uint64(9+i)*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	}

	// Try to get another slice while we're already at the limit.
	_, err = get(1)
	require.ErrorContains(t, err, expectedError)
	require.Equal(t, 11*elementSize, pool.CurrentEstimatedMemoryConsumptionBytes)
	require.Equal(t, 11*elementSize, pool.PeakEstimatedMemoryConsumptionBytes)
	assertRejectedQueryCount(t, reg, 1)
}

func assertRejectedQueryCount(t *testing.T, reg *prometheus.Registry, expectedRejectionCount int) {
	expected := fmt.Sprintf(`
		# TYPE %s counter
		%s %v
	`, rejectedQueryName, rejectedQueryName, expectedRejectionCount)
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), rejectedQueryName))
}

func TestLimitingPool_ClearsReturnedSlices(t *testing.T) {
	pool := NewLimitingPool(0, nil)

	// Get a slice, put it back in the pool and get it back again.
	// Make sure all elements are zero or false when we get it back.
	t.Run("[]float64", func(t *testing.T) {
		floatSlice, err := pool.GetFloatSlice(2)
		require.NoError(t, err)
		floatSlice = floatSlice[:2]
		floatSlice[0] = 123
		floatSlice[1] = 456

		pool.PutFloatSlice(floatSlice)

		floatSlice, err = pool.GetFloatSlice(2)
		require.NoError(t, err)
		floatSlice = floatSlice[:2]
		require.Equal(t, []float64{0, 0}, floatSlice)
	})

	t.Run("[]bool", func(t *testing.T) {
		boolSlice, err := pool.GetBoolSlice(2)
		require.NoError(t, err)
		boolSlice = boolSlice[:2]
		boolSlice[0] = false
		boolSlice[1] = true

		pool.PutBoolSlice(boolSlice)

		boolSlice, err = pool.GetBoolSlice(2)
		require.NoError(t, err)
		boolSlice = boolSlice[:2]
		require.Equal(t, []bool{false, false}, boolSlice)
	})
}

func createRejectedMetric() (*prometheus.Registry, prometheus.Counter) {
	reg := prometheus.NewPedanticRegistry()
	metric := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: rejectedQueryName,
	})

	return reg, metric
}
