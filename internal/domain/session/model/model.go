package model

// Session — вошедший человек, извлечённый из JWT (ADR-005). Stateless: в БД не хранится.
type Session struct {
	Id    int64
	Admin bool
}

func New(id int64) *Session {
	return &Session{Id: id}
}

func (s *Session) IsAuthorized() bool {
	return s != nil && s.Id > 0
}

func (s *Session) IsAdmin() bool {
	return s.IsAuthorized() && s.Admin
}
