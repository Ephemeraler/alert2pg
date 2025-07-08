package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mockAlertmanagerServer(t *testing.T, statusCode int, alerts Alerts) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			require.NoError(t, json.NewEncoder(w).Encode(alerts))
		}
	})
	return httptest.NewServer(handler)
}

func TestGetFiringAlertsFromAlertmanager_Success(t *testing.T) {
	alertsData := Alerts{
		{
			Fingerprint:  "abc123",
			StartsAt:     time.Now().Add(-time.Hour),
			EndsAt:       time.Now().Add(time.Hour),
			Labels:       map[string]string{"severity": "critical"},
			Annotations:  map[string]string{"summary": "test alert"},
			GeneratorURL: "http://example.com",
		},
	}
	server := mockAlertmanagerServer(t, http.StatusOK, alertsData)
	defer server.Close()

	addr := server.Listener.Addr().String()
	got, err := GetFiringAlertsFromAlertmanager(addr, true, false, false, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "abc123", got[0].Fingerprint)
	require.Equal(t, "critical", got[0].Labels["severity"])
	require.Equal(t, "test alert", got[0].Annotations["summary"])
}

func TestGetFiringAlertsFromAlertmanager_HTTPError(t *testing.T) {
	server := mockAlertmanagerServer(t, http.StatusInternalServerError, nil)
	defer server.Close()

	addr := server.Listener.Addr().String()
	got, err := GetFiringAlertsFromAlertmanager(addr, true, false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "请求失败")
	require.Len(t, got, 0)
}

func TestGetFiringAlertsFromAlertmanager_InvalidJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{invalid json}")
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	addr := server.Listener.Addr().String()
	got, err := GetFiringAlertsFromAlertmanager(addr, true, false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无法解析响应体")
	require.Len(t, got, 0)
}

func TestGetFiringAlertsFromAlertmanager_ConnectionError(t *testing.T) {
	// Use an invalid port to force connection error
	addr := "127.0.0.1:0"
	got, err := GetFiringAlertsFromAlertmanager(addr, true, false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无法发送请求")
	require.Len(t, got, 0)
}
