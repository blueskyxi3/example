package hostclient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"ai-hub-example/hostclient"
)

func TestDataCallbackOverrideIsRequestScoped(t *testing.T) {
	client, err := hostclient.New("http://master:3000", "")
	require.NoError(t, err)
	first, err := hostclient.WithDataHostBaseURL(context.Background(), "http://slave-a:3001")
	require.NoError(t, err)
	second, err := hostclient.WithDataHostBaseURL(context.Background(), "http://slave-b:3002")
	require.NoError(t, err)
	require.Equal(t, "http://slave-a:3001", client.DataHostBaseURLFor(first))
	require.Equal(t, "http://slave-b:3002", client.DataHostBaseURLFor(second))
	require.Equal(t, "http://master:3000", client.HostBaseURL())
	require.Equal(t, "http://master:3000", client.DataHostBaseURL())
	_, err = hostclient.WithDataHostBaseURL(context.Background(), "http://slave/path")
	require.Error(t, err)
}
