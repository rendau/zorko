package model

import "time"

// Event — событие SDK, приведённое к единому виду (после вычистки чувствительных данных)
type Event struct {
	EventId    string
	ProjectId  int64
	IssueId    int64 // заполняется после группировки
	Timestamp  time.Time
	ReceivedAt time.Time

	Level       string
	Platform    string
	Release     string
	Environment string
	Transaction string
	ServerName  string

	// Message — итоговый текст сообщения; MessageTemplate — шаблон без подстановки params (для отпечатка)
	Message         string
	MessageTemplate string

	// Exceptions — цепочка исключений в порядке SDK: последнее — то, что поймали снаружи
	Exceptions  []*Exception
	Fingerprint []string

	User       User
	RequestUrl string

	Browser     string // «Chrome 130.0»
	BrowserName string
	Os          string // «macOS 15.1»
	OsName      string
	Device      string
	Runtime     string // «node v22.1.0»

	SdkName    string
	SdkVersion string

	Tags map[string]string

	// Data — всё событие JSON-ом после вычистки (хранится в ClickHouse как есть)
	Data []byte

	Grouping Grouping
}

type Exception struct {
	Type      string
	Value     string
	Module    string
	Mechanism Mechanism
	Frames    []*Frame // в порядке SDK: последний кадр — место падения
}

type Mechanism struct {
	Type    string
	Handled *bool
	// Synthetic — исключение собрано SDK для сообщения со стеком (captureMessage): тип и текст не настоящие
	Synthetic bool
}

type Frame struct {
	Filename string
	AbsPath  string
	Module   string
	Package  string
	Function string
	Lineno   int
	Colno    int
	InApp    *bool
}

type User struct {
	Id        string
	Email     string
	Username  string
	IpAddress string
}

// Grouping — результат группировки (ADR-004)
type Grouping struct {
	Hash    string
	Version string
	Title   string
	Culprit string
	Type    string // тип верхнего исключения
	Value   string // первая строка его текста
}

// UserKey — по кому считать «уникальных людей»: id, почта, имя, IP
func (u User) Key() string {
	for _, v := range []string{u.Id, u.Email, u.Username, u.IpAddress} {
		if v != "" {
			return v
		}
	}
	return ""
}
