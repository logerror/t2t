package agent

type Terminal interface {
	StartShell() error
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(rows, cols int) error
	Close() error
}
