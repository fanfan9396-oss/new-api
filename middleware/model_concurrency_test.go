package middleware

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestModelConcurrencyAdmissionLimitsUserAndGlobal(t *testing.T) {
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	resetModelConcurrencyForTest()

	releases := make([]func(), 0, 5)
	for i := 0; i < 5; i++ {
		release, ok := admitModelConcurrency(context.Background(), 4101, 5, 32)
		require.True(t, ok)
		releases = append(releases, release)
	}
	_, ok := admitModelConcurrency(context.Background(), 4101, 5, 32)
	require.False(t, ok, "sixth request for one user must be rejected")
	for _, release := range releases {
		release()
	}

	globalReleases := make([]func(), 0, 32)
	for i := 0; i < 32; i++ {
		release, ok := admitModelConcurrency(context.Background(), 5000+i, 5, 32)
		require.True(t, ok)
		globalReleases = append(globalReleases, release)
	}
	_, ok = admitModelConcurrency(context.Background(), 5999, 5, 32)
	require.False(t, ok, "global concurrency limit must reject the 33rd request")
	for _, release := range globalReleases {
		release()
	}

	_, ok = admitModelConcurrency(context.Background(), 5999, 5, 32)
	require.True(t, ok, "released global slots must be reusable")
}

func TestModelConcurrencyAdmissionLimitsRedisAndReleases(t *testing.T) {
	_, _ = useRateLimitMiniRedis(t)
	releases := make([]func(), 0, 2)
	for i := 0; i < 2; i++ {
		release, ok := admitModelConcurrency(context.Background(), 4201, 2, 2)
		require.True(t, ok)
		releases = append(releases, release)
	}
	_, ok := admitModelConcurrency(context.Background(), 4201, 2, 2)
	require.False(t, ok)
	releases[0]()
	_, ok = admitModelConcurrency(context.Background(), 4201, 2, 2)
	require.True(t, ok)
	releases[1]()
}
