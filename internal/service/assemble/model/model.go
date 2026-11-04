package model

// Request — просьба sentry-cli собрать архив из кусков (artifactbundle/assemble)
type Request struct {
	OrgId      int64
	Checksum   string
	Chunks     []string
	ProjectIds []int64
	Release    string
	Dist       string
}

// Состояния ответа — как у Sentry (ChunkedFileState)
const (
	StateOk         = "ok"
	StateCreated    = "created"
	StateAssembling = "assembling"
	StateNotFound   = "not_found"
	StateError      = "error"
)

type Result struct {
	State         string
	MissingChunks []string
	Detail        string
}
