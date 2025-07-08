package alert

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
)

func TestAlert(t *testing.T) {
	content := `
	{
		"receiver": "alertsnitch",
		"status": "resolved",
		"alerts": [
			{
				"status": "resolved",
				"labels": {
					"alertname": "MyAlertName",
					"hostname": "myhostname",
					"job": "node-exporter",
					"severity": "critical",
					"tier": "svc"
				},
				"annotations": {
					"summary": "Non ephemeral host is DOWN"
				},
				"startsAt": "2019-01-02T10:31:46.054Z",
				"endsAt": "2019-01-02T10:36:46.054Z",
				"generatorURL": "http://prometheus.int/graph?g0.expr=up%7Bjob%3D%22node-exporter%22%2Ctier%21%3D%22ephemeral%22%7D+%3D%3D+0&g0.tab=1",
				"fingerprint": "dd19ae3d4e06ac55"
			},
			{
				"annotations": {
				"description": "节点可用率低于90%",
				"summary": "节点可用率低于90%"
				},
				"endsAt": "2025-07-08T06:05:48.268Z",
				"fingerprint": "077bf4e884599215",
				"startsAt": "2025-07-02T22:23:18.268Z",
				"status": "firing",
				"generatorURL": "/graph?g0.expr=cluster_availability%7Bcluster%3D%22test%22%2Csource%3D%22other%22%7D+%3C+0.9&g0.tab=1",
				"labels": {
					"alertname": "clusterAvailabilityLow",
					"cluster": "test",
					"severity": "INFO",
					"source": "other",
					"type": "SERVICE"
				}
			}
		],
		"groupLabels": {
			"alertname": "NonEphemeralHostIsDown"
		},
		"commonLabels": {
			"alertname": "NonEphemeralHostIsDown",
			"hostname": "myhostname",
			"job": "node-exporter",
			"severity": "critical",
			"tier": "svc"
		},
		"commonAnnotations": {
			"host_tier": "myhostname",
			"summary": "Non ephemeral host is DOWN"
		},
		"externalURL": "http://alertmanager:9093",
		"version": "4",
		"groupKey": "{}/{}:{alertname=\"NonEphemeralHostIsDown\"}"
	}
	`

	var group AlertGroup
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&group); err != nil {
		t.Fatalf("failed to decode alert group: %v", err)
	}

	epected := AlertGroup{
		Version:  "4",
		GroupKey: "{}/{}:{alertname=\"NonEphemeralHostIsDown\"}",
		Receiver: "alertsnitch",
		Status:   "resolved",
		Alerts: Alerts{
			{
				Loaded:   false,
				LoadedAt: time.Now(),
				Status:   "resolved",
				Labels: map[string]string{
					"alertname": "MyAlertName",
					"hostname":  "myhostname",
					"job":       "node-exporter",
					"severity":  "critical",
					"tier":      "svc",
				},
				Annotations: map[string]string{
					"summary": "Non ephemeral host is DOWN",
				},
				StartsAt:     time.Date(2019, 1, 2, 10, 31, 46, 54000000, time.UTC),
				EndsAt:       time.Date(2019, 1, 2, 10, 36, 46, 54000000, time.UTC),
				GeneratorURL: "http://prometheus.int/graph?g0.expr=up%7Bjob%3D%22node-exporter%22%2Ctier%21%3D%22ephemeral%22%7D+%3D%3D+0&g0.tab=1",
				Fingerprint:  "dd19ae3d4e06ac55",
			},
			{
				Loaded:   false,
				LoadedAt: time.Now(),
				Annotations: map[string]string{
					"description": "节点可用率低于90%",
					"summary":     "节点可用率低于90%",
				},
				EndsAt:       time.Date(2025, 7, 8, 6, 5, 48, 268000000, time.UTC),
				Fingerprint:  "077bf4e884599215",
				StartsAt:     time.Date(2025, 7, 2, 22, 23, 18, 268000000, time.UTC),
				Status:       "firing",
				GeneratorURL: "/graph?g0.expr=cluster_availability%7Bcluster%3D%22test%22%2Csource%3D%22other%22%7D+%3C+0.9&g0.tab=1",
				Labels: map[string]string{
					"alertname": "clusterAvailabilityLow",
					"cluster":   "test",
					"severity":  "INFO",
					"source":    "other",
					"type":      "SERVICE",
				},
			},
		},
		GroupLabels: map[string]string{
			"alertname": "NonEphemeralHostIsDown",
		},
		CommonLabels: map[string]string{
			"alertname": "NonEphemeralHostIsDown",
			"hostname":  "myhostname",
			"job":       "node-exporter",
			"severity":  "critical",
			"tier":      "svc",
		},
		CommonAnnotations: map[string]string{
			"host_tier": "myhostname",
			"summary":   "Non ephemeral host is DOWN",
		},
		ExternalURL: "http://alertmanager:9093",
	}

	if diff := cmp.Diff(epected, group, cmpopts.IgnoreFields(AlertGroup{}, "Alerts")); diff != "" {
		t.Errorf("AlertGroup mismatch (-want +got):\n%s", diff)
	}

	epectedAlerts := epected.Alerts
	sort.Slice(epectedAlerts, func(i, j int) bool {
		return epectedAlerts[i].Fingerprint < epectedAlerts[j].Fingerprint
	})
	actualAlerts := group.Alerts
	sort.Slice(actualAlerts, func(i, j int) bool {
		return actualAlerts[i].Fingerprint < actualAlerts[j].Fingerprint
	})
	require.Equal(t, len(epectedAlerts), len(actualAlerts))
	for i := 0; i < len(epectedAlerts); i++ {
		require.False(t, actualAlerts[i].Loaded)
		require.True(t, actualAlerts[i].LoadedAt.Before(epectedAlerts[i].LoadedAt))
		require.True(t, actualAlerts[i].Equal(epectedAlerts[i]))
	}
}

func TestIsExpired(t *testing.T) {
	alert := Alert{
		Loaded:   false,
		Status:   Resolved,
		LoadedAt: time.Now(),
	}
	require.False(t, alert.IsExpired(time.Minute))

	alert = Alert{
		Loaded:   true,
		Status:   Resolved,
		LoadedAt: time.Now().Add(-time.Hour),
	}
	require.True(t, alert.IsExpired(time.Minute))

	alert = Alert{
		Loaded:   true,
		Status:   Firing,
		LoadedAt: time.Now().Add(-time.Hour),
	}
	require.False(t, alert.IsExpired(time.Minute))
}

func TestSetResolved(t *testing.T) {
	alert := Alert{
		Loaded:   true,
		LoadedAt: time.Time{},
		Status:   Firing,
		EndsAt:   time.Time{},
	}

	alert.SetResolved()
	require.False(t, alert.Loaded)
	require.False(t, alert.LoadedAt.IsZero())
	require.Equal(t, Resolved, alert.Status)
	require.False(t, alert.EndsAt.IsZero())
}
