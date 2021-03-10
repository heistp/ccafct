package unit

// Bytes is a number of bytes.
type Bytes int64

const (
	Byte     Bytes = 1
	Kilobyte       = 1024 * Byte
	Megabyte       = 1024 * Kilobyte
	Gigabyte       = 1024 * Megabyte
	Terabyte       = 1024 * Gigabyte
)
