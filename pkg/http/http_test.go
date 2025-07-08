package http

import (
	"alert2pg/pkg/alert"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mockAlertmanagerServer(t *testing.T, statusCode int, content string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			_, err := w.Write([]byte(content))
			require.NoError(t, err, "写入响应内容失败")
		}
	})
	return httptest.NewServer(handler)
}

func TestGetFiringAlertsFromAlertmanager_Success(t *testing.T) {
	content := `
	[
		{
			"annotations": {
			"description": "节点可用率低于90%",
			"summary": "节点可用率低于90%"
			},
			"endsAt": "2025-07-08T06:05:48.268Z",
			"fingerprint": "077bf4e884599215",
			"receivers": [
			{
				"name": "web_hook_default"
			}
			],
			"startsAt": "2025-07-02T22:23:18.268Z",
			"status": {
			"inhibitedBy": [],
			"silencedBy": [],
			"state": "active"
			},
			"updatedAt": "2025-07-08T06:01:48.609Z",
			"generatorURL": "/graph?g0.expr=cluster_availability%7Bcluster%3D%22test%22%2Csource%3D%22other%22%7D+%3C+0.9&g0.tab=1",
			"labels": {
			"alertname": "clusterAvailabilityLow",
			"cluster": "test",
			"severity": "INFO",
			"source": "other",
			"type": "SERVICE"
			}
		},
		{
			"annotations": {
			"description": "0008卷降级",
			"summary": "0008卷降级"
			},
			"endsAt": "2025-07-08T06:05:48.268Z",
			"fingerprint": "0de75c943e4d50f9",
			"receivers": [
			{
				"name": "web_hook_default"
			}
			],
			"startsAt": "2025-07-07T10:23:03.268Z",
			"status": {
			"inhibitedBy": [],
			"silencedBy": [],
			"state": "active"
			},
			"updatedAt": "2025-07-08T06:01:48.595Z",
			"generatorURL": "/graph?g0.expr=lustre_volume_degraded%7Bcluster%3D%22test%22%2Cdevice%3D%22oss1%22%2Cvolume%3D%220008%22%7D+%3E+0&g0.tab=1",
			"labels": {
			"alertname": "lustreDegraded",
			"cluster": "test",
			"group": "storage",
			"job": "9275_15s",
			"severity": "SEVERITY",
			"source": "storage",
			"type": "SERVICE",
			"volume": "0008"
			}
		}
	]
	`
	expected := alert.Alerts{
		{
			Loaded:      false,
			LoadedAt:    time.Now(),
			Fingerprint: "077bf4e884599215",
			Status:      alert.Firing,
			StartsAt:    time.Date(2025, 7, 2, 22, 23, 18, 268000000, time.UTC),
			EndsAt:      time.Date(2025, 7, 8, 6, 5, 48, 268000000, time.UTC),
			Labels: map[string]string{
				"alertname": "clusterAvailabilityLow",
				"cluster":   "test",
				"severity":  "INFO",
				"source":    "other",
				"type":      "SERVICE",
			},
			Annotations: map[string]string{
				"description": "节点可用率低于90%",
				"summary":     "节点可用率低于90%",
			},
			GeneratorURL: "/graph?g0.expr=cluster_availability%7Bcluster%3D%22test%22%2Csource%3D%22other%22%7D+%3C+0.9&g0.tab=1",
		},
		{
			Loaded:      false,
			LoadedAt:    time.Now(),
			Fingerprint: "0de75c943e4d50f9",
			Status:      alert.Firing,
			StartsAt:    time.Date(2025, 7, 7, 10, 23, 3, 268000000, time.UTC),
			EndsAt:      time.Date(2025, 7, 8, 6, 5, 48, 268000000, time.UTC),
			Labels: map[string]string{
				"alertname": "lustreDegraded",
				"cluster":   "test",
				"group":     "storage",
				"job":       "9275_15s",
				"severity":  "SEVERITY",
				"source":    "storage",
				"type":      "SERVICE",
				"volume":    "0008",
			},
			Annotations: map[string]string{
				"description": "0008卷降级",
				"summary":     "0008卷降级",
			},
			GeneratorURL: "/graph?g0.expr=lustre_volume_degraded%7Bcluster%3D%22test%22%2Cdevice%3D%22oss1%22%2Cvolume%3D%220008%22%7D+%3E+0&g0.tab=1",
		},
	}
	server := mockAlertmanagerServer(t, http.StatusOK, content)
	defer server.Close()

	addr := server.Listener.Addr().String()
	actual, err := GetFiringAlertsFromAlertmanager(addr, true, false, false, false)
	require.NoError(t, err)
	require.Len(t, actual, 2)
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Fingerprint < expected[j].Fingerprint
	})
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].Fingerprint < actual[j].Fingerprint
	})
	for i := 0; i < len(actual); i++ {
		require.False(t, actual[i].Loaded)
		require.True(t, actual[i].LoadedAt.After(expected[i].LoadedAt))
		require.True(t, actual[i].Equal(expected[i]))
	}
}
