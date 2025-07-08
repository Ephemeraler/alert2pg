package alert

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	Firing   = "firing"   // 报警状态: Firing
	Resolved = "resolved" // 报警状态: Resolved
)

func DefaultAlert() Alert {
	return Alert{
		Loaded:   false,
		LoadedAt: time.Now(),
	}
}

type AlertGroup struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Receiver          string            `json:"receiver"`
	Status            string            `json:"status"`
	Alerts            Alerts            `json:"alerts"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
}

type Alerts []Alert

type Alert struct {
	// 标记位, 表示报警信息是否已经写入 DB.
	Loaded bool `json:"-"`

	// 当 Loaded 标记位为 True 时, 该字段表示报警被"加载"进数据库的时间.
	// 加载有两层含义:
	// 1) 由 storage 任务将报警信息成功写入数据库
	// 2) 当 loaded 为 True 时, 重复接收并"逻辑层面(非真实写入)"将该报警信息成功写入到数据库的时间.
	LoadedAt time.Time `json:"-"`

	// 报警信息
	Fingerprint  string            `json:"fingerprint"`
	Status       string            `json:"status"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
}

// UnmarshalJSON 实现自定义的 JSON 反序列化方法, 确保反序列化时标记字段被初始化.
func (a *Alert) UnmarshalJSON(data []byte) error {
	type plain Alert
	*a = DefaultAlert()
	return json.Unmarshal(data, (*plain)(a))
}

func (a *Alert) Key() string {
	return fmt.Sprintf("%s:%d", a.Fingerprint, a.StartsAt.UnixMilli())
}

// Equal 判断报警内容是否一致.
func (a Alert) Equal(b Alert) bool {
	if a.Fingerprint != b.Fingerprint ||
		a.Status != b.Status ||
		a.StartsAt != b.StartsAt ||
		a.EndsAt != b.EndsAt ||
		a.GeneratorURL != b.GeneratorURL ||
		len(a.Annotations) != len(b.Annotations) {
		return false
	}

	for k, v := range a.Labels {
		if bVal, ok := b.Labels[k]; !ok || v != bVal {
			return false
		}
	}

	return true
}

// IsExpired 判断 Resolved 报警信息是否过期.
func (a Alert) IsExpired(t time.Duration) bool {
	//
	return a.Status == Resolved && a.Loaded && time.Since(a.LoadedAt) > t
}

// SetResolved 设置报警状态为 Resolved.
func (a *Alert) SetResolved() {
	a.Loaded = false
	a.Status = Resolved
	a.EndsAt = time.Now()
	a.LoadedAt = time.Now()
}
