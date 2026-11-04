package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mileusna/useragent"
	"github.com/samber/lo"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	localModel "github.com/rendau/zorko/internal/service/normalize/service/model"
)

const (
	maxTags        = 200
	maxTagKeyLen   = 32
	maxTagValueLen = 200

	// время события вне окна [приём − pastWindow, приём + futureWindow] — кривые часы устройства
	pastWindow   = 30 * 24 * time.Hour
	futureWindow = time.Minute
)

type Service struct{}

func New() *Service {
	return &Service{}
}

// Normalize разбирает событие SDK; ошибка — только если тело не JSON-объект
func (s *Service) Normalize(raw *eventModel.RawEvent) (*eventModel.Event, error) {
	var tree map[string]any
	if err := json.Unmarshal(raw.Payload, &tree); err != nil {
		return nil, fmt.Errorf("event payload: %w", err)
	}

	scrub(tree)

	ipAuto := false
	if user, ok := tree["user"].(map[string]any); ok && user["ip_address"] == "{{auto}}" {
		user["ip_address"] = raw.ClientIp
		ipAuto = true
	}

	data, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal(scrubbed): %w", err)
	}

	var rep localModel.EventRep
	if err = json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("event payload: %w", err)
	}

	ev := &eventModel.Event{
		EventId:     lo.CoalesceOrEmpty(normalizeEventId(rep.EventId), raw.EventId),
		ProjectId:   raw.ProjectId,
		ReceivedAt:  raw.ReceivedAt,
		Timestamp:   clampTimestamp(rep.Timestamp.Time, raw.ReceivedAt),
		Level:       lo.CoalesceOrEmpty(strings.ToLower(rep.Level), "error"),
		Platform:    lo.CoalesceOrEmpty(rep.Platform, "other"),
		Release:     rep.Release,
		Dist:        rep.Dist,
		Environment: lo.CoalesceOrEmpty(rep.Environment, "production"),
		Transaction: rep.Transaction,
		ServerName:  rep.ServerName,
		Exceptions:  lo.Map(rep.Exception, decodeException),
		Fingerprint: localModel.FingerprintValues(rep.Fingerprint),
		Data:        data,
	}
	if ev.Level == "warn" {
		ev.Level = "warning"
	}

	// сообщение: logentry (новые SDK) или message (старые); шаблон — без подстановки params
	msg := rep.Message
	if rep.Logentry != nil {
		msg = *rep.Logentry
	}
	ev.Message = lo.CoalesceOrEmpty(msg.Formatted, msg.Message)
	ev.MessageTemplate = lo.Ternary(msg.Message != "" && msg.Params != nil, msg.Message, ev.Message)

	if rep.User != nil {
		ev.User = eventModel.User{
			Id:        string(rep.User.Id),
			Email:     rep.User.Email,
			Username:  rep.User.Username,
			IpAddress: lo.Ternary(ipAuto || rep.User.IpAddress != "", rep.User.IpAddress, ""),
		}
	}
	userAgent := ""
	if rep.Request != nil {
		ev.RequestUrl = rep.Request.Url
		for k, v := range rep.Request.Headers {
			if strings.EqualFold(k, "User-Agent") {
				userAgent = v
			}
		}
	}
	if c := rep.Contexts.Browser; c != nil {
		ev.BrowserName = c.Name
		ev.Browser = joinNonEmpty(c.Name, c.Version)
	}
	if c := rep.Contexts.Os; c != nil {
		ev.OsName = c.Name
		ev.Os = joinNonEmpty(c.Name, c.Version)
	}
	if c := rep.Contexts.Runtime; c != nil {
		ev.Runtime = joinNonEmpty(c.Name, c.Version)
	}
	if c := rep.Contexts.Device; c != nil {
		ev.Device = lo.CoalesceOrEmpty(c.Model, c.Family, c.Brand)
	}
	// браузерные SDK не присылают browser/os: их, как и Sentry, вычисляем из User-Agent
	if userAgent != "" && (ev.Browser == "" || ev.Os == "") {
		applyUserAgent(ev, userAgent)
	}
	if rep.Sdk != nil {
		ev.SdkName = rep.Sdk.Name
		ev.SdkVersion = rep.Sdk.Version
	}

	ev.Tags = buildTags(ev, rep.Tags)

	if rep.DebugMeta != nil {
		for _, img := range rep.DebugMeta.Images {
			if img.Type == "sourcemap" && img.CodeFile != "" && img.DebugId != "" {
				if ev.DebugImages == nil {
					ev.DebugImages = make(map[string]string, len(rep.DebugMeta.Images))
				}
				ev.DebugImages[img.CodeFile] = img.DebugId
			}
		}
	}

	return ev, nil
}

func decodeException(v localModel.ExceptionRep, _ int) *eventModel.Exception {
	result := &eventModel.Exception{
		Type:   v.Type,
		Value:  string(v.Value),
		Module: v.Module,
	}
	if v.Mechanism != nil {
		result.Mechanism = eventModel.Mechanism{
			Type:      v.Mechanism.Type,
			Handled:   v.Mechanism.Handled,
			Synthetic: v.Mechanism.Synthetic,
		}
	}
	if v.Stacktrace != nil {
		result.Frames = lo.Map(v.Stacktrace.Frames, func(f localModel.FrameRep, _ int) *eventModel.Frame {
			return &eventModel.Frame{
				Filename: f.Filename,
				AbsPath:  f.AbsPath,
				Module:   f.Module,
				Package:  f.Package,
				Function: f.Function,
				Lineno:   f.Lineno,
				Colno:    f.Colno,
				InApp:    f.InApp,
			}
		})
	}
	return result
}

// buildTags — теги SDK плюс производные, как в Sentry; с пределами на число и длину
func buildTags(ev *eventModel.Event, sdkTags localModel.Tags) map[string]string {
	tags := make(map[string]string, len(sdkTags)+12)

	set := func(k, v string) {
		if v == "" || len(k) > maxTagKeyLen || len(tags) >= maxTags {
			return
		}
		tags[k] = truncate(v, maxTagValueLen)
	}

	for k, v := range sdkTags {
		set(k, v)
	}

	set("level", ev.Level)
	set("environment", ev.Environment)
	set("release", ev.Release)
	set("browser", ev.Browser)
	set("browser.name", ev.BrowserName)
	set("os", ev.Os)
	set("os.name", ev.OsName)
	set("device", ev.Device)
	set("runtime", ev.Runtime)
	set("url", ev.RequestUrl)
	set("transaction", ev.Transaction)

	if len(ev.Exceptions) > 0 {
		top := ev.Exceptions[len(ev.Exceptions)-1]
		set("mechanism", top.Mechanism.Type)
		if top.Mechanism.Handled != nil {
			set("handled", lo.Ternary(*top.Mechanism.Handled, "yes", "no"))
		}
	}

	switch {
	case ev.User.Id != "":
		set("user", "id:"+ev.User.Id)
	case ev.User.Email != "":
		set("user", "email:"+ev.User.Email)
	case ev.User.Username != "":
		set("user", "username:"+ev.User.Username)
	case ev.User.IpAddress != "":
		set("user", "ip:"+ev.User.IpAddress)
	}

	return tags
}

func applyUserAgent(ev *eventModel.Event, userAgent string) {
	ua := useragent.Parse(userAgent)
	if ev.Browser == "" && ua.Name != "" {
		ev.BrowserName = ua.Name
		ev.Browser = joinNonEmpty(ua.Name, majorMinor(ua.Version))
	}
	if ev.Os == "" && ua.OS != "" {
		ev.OsName = ua.OS
		ev.Os = joinNonEmpty(ua.OS, ua.OSVersion)
	}
	if ev.Device == "" {
		ev.Device = ua.Device
	}
}

// majorMinor — «130.0.6723.91» → «130.0»: полная сборка браузера дробит распределение по версиям
func majorMinor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	return strings.Join(parts[:min(len(parts), 2)], ".")
}

func clampTimestamp(ts, receivedAt time.Time) time.Time {
	if ts.IsZero() || ts.Before(receivedAt.Add(-pastWindow)) || ts.After(receivedAt.Add(futureWindow)) {
		return receivedAt
	}
	return ts
}

func normalizeEventId(v string) string {
	v = strings.ToLower(strings.ReplaceAll(v, "-", ""))
	if len(v) != 32 || strings.Trim(v, "0123456789abcdef") != "" {
		return ""
	}
	return v
}

func joinNonEmpty(parts ...string) string {
	return strings.Join(lo.Compact(parts), " ")
}

// truncate — не длиннее n байт, не разрезая символ UTF-8
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
