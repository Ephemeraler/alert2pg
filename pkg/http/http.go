package http

import (
	"alert2pg/pkg/alert"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Alert struct {
	Fingerprint  string            `json:"fingerprint"`
	Status       string            `json:"-"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
}

func (a *Alert) UnmarshalJSON(data []byte) error {
	type plain Alert
	*a = Alert{
		Status:      alert.Firing,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
	return json.Unmarshal(data, (*plain)(a))
}

type Alerts []Alert

// GetFiringAlertsFromAlertmanager 从 Alertmanager 获取当前处于 Firing 状态的报警信息.
func GetFiringAlertsFromAlertmanager(addr string, active, silenced, inhibited, unprocessed bool) (alert.Alerts, error) {
	alerts := make(alert.Alerts, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s/api/v2/alerts?active=%t&silenced=%t&inhibited=%t&unprocessed=%t", addr, active, silenced, inhibited, unprocessed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return alerts, fmt.Errorf("无法创建请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return alerts, fmt.Errorf("无法发送请求: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return alerts, fmt.Errorf("请求失败: %s", resp.Status)
	}
	var rlt Alerts
	if err := json.NewDecoder(resp.Body).Decode(&rlt); err != nil {
		return alerts, fmt.Errorf("无法解析响应体: %w", err)
	}

	for _, a := range rlt {
		alerts = append(alerts, alert.Alert{
			Fingerprint:  a.Fingerprint,
			Status:       a.Status,
			StartsAt:     a.StartsAt,
			EndsAt:       a.EndsAt,
			Labels:       a.Labels,
			Annotations:  a.Annotations,
			GeneratorURL: a.GeneratorURL,
			Loaded:       false,
			LoadedAt:     time.Now(),
		})
	}

	return alerts, nil
}
