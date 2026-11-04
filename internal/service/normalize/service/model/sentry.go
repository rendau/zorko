package model

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Событие SDK Sentry (develop.sentry.dev/sdk/data-model/event-payloads) — только поля, которые
// нужны zorko; всё событие целиком хранится отдельно. У части полей несколько допустимых форм.

type EventRep struct {
	EventId     string            `json:"event_id"`
	Timestamp   Timestamp         `json:"timestamp"`
	Level       string            `json:"level"`
	Platform    string            `json:"platform"`
	Release     string            `json:"release"`
	Dist        string            `json:"dist"`
	Environment string            `json:"environment"`
	Transaction string            `json:"transaction"`
	ServerName  string            `json:"server_name"`
	Message     Message           `json:"message"`
	Logentry    *Message          `json:"logentry"`
	Exception   ExceptionList     `json:"exception"`
	Fingerprint []json.RawMessage `json:"fingerprint"`
	Tags        Tags              `json:"tags"`
	User        *UserRep          `json:"user"`
	Request     *RequestRep       `json:"request"`
	Contexts    ContextsRep       `json:"contexts"`
	Sdk         *SdkRep           `json:"sdk"`
	DebugMeta   *DebugMetaRep     `json:"debug_meta"`
}

// DebugMetaRep — метки сжатых файлов (images type=sourcemap: code_file + debug_id)
type DebugMetaRep struct {
	Images []DebugImageRep `json:"images"`
}

type DebugImageRep struct {
	Type     string `json:"type"`
	CodeFile string `json:"code_file"`
	DebugId  string `json:"debug_id"`
}

// Timestamp — секунды с дробной частью или строка RFC 3339
type Timestamp struct {
	time.Time
}

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		sec := int64(f)
		t.Time = time.Unix(sec, int64((f-float64(sec))*1e9)).UTC()
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
			if parsed, err := time.Parse(layout, s); err == nil {
				t.Time = parsed.UTC()
				return nil
			}
		}
	}
	return nil // кривое время не роняет событие: возьмём время приёма
}

// Message — строка или {message, formatted, params}
type Message struct {
	Message   string `json:"message"`
	Formatted string `json:"formatted"`
	Params    any    `json:"params"`
}

func (m *Message) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &m.Formatted)
	}
	type plain Message
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	*m = Message(v)
	return nil
}

// ExceptionList — {values: [...]} или сразу [...]
type ExceptionList []ExceptionRep

func (l *ExceptionList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var values []ExceptionRep
		if err := json.Unmarshal(b, &values); err != nil {
			return nil
		}
		*l = values
		return nil
	}
	var v struct {
		Values []ExceptionRep `json:"values"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	*l = v.Values
	return nil
}

type ExceptionRep struct {
	Type       string         `json:"type"`
	Value      Stringish      `json:"value"`
	Module     string         `json:"module"`
	Mechanism  *MechanismRep  `json:"mechanism"`
	Stacktrace *StacktraceRep `json:"stacktrace"`
}

type MechanismRep struct {
	Type      string `json:"type"`
	Handled   *bool  `json:"handled"`
	Synthetic bool   `json:"synthetic"`
}

type StacktraceRep struct {
	Frames []FrameRep `json:"frames"`
}

type FrameRep struct {
	Filename string `json:"filename"`
	AbsPath  string `json:"abs_path"`
	Module   string `json:"module"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Lineno   int    `json:"lineno"`
	Colno    int    `json:"colno"`
	InApp    *bool  `json:"in_app"`
}

// Tags — {k: v} или [[k, v], ...]; значения любого типа приводятся к строке
type Tags map[string]string

func (t *Tags) UnmarshalJSON(b []byte) error {
	result := Tags{}

	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err == nil {
		for k, v := range obj {
			result[k] = stringify(v)
		}
		*t = result
		return nil
	}

	var pairs [][]any
	if err := json.Unmarshal(b, &pairs); err == nil {
		for _, p := range pairs {
			if len(p) == 2 {
				result[stringify(p[0])] = stringify(p[1])
			}
		}
	}
	*t = result
	return nil
}

type UserRep struct {
	Id        Stringish `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	IpAddress string    `json:"ip_address"`
}

type RequestRep struct {
	Url     string `json:"url"`
	Headers Tags   `json:"headers"` // объект или список пар — как теги
}

type ContextsRep struct {
	Browser *NameVersionRep `json:"browser"`
	Os      *NameVersionRep `json:"os"`
	Runtime *NameVersionRep `json:"runtime"`
	Device  *DeviceRep      `json:"device"`
}

type NameVersionRep struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type DeviceRep struct {
	Model  string `json:"model"`
	Family string `json:"family"`
	Brand  string `json:"brand"`
}

type SdkRep struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Stringish — строка, число или что угодно ещё — как строка
type Stringish string

func (s *Stringish) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	*s = Stringish(stringify(v))
	return nil
}

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return strings.TrimSpace(string(b))
	}
}

// FingerprintValues — элементы fingerprint строками
func FingerprintValues(v []json.RawMessage) []string {
	result := make([]string, 0, len(v))
	for _, raw := range v {
		var x any
		if err := json.Unmarshal(raw, &x); err == nil {
			result = append(result, stringify(x))
		}
	}
	return result
}
