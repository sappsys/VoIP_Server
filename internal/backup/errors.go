package backup

import "errors"

var (
	ErrUnsupportedFormat = errors.New("unsupported backup format version")
	ErrInvalidManifest   = errors.New("invalid backup manifest")
	ErrMissingFile       = errors.New("backup missing required file")
	ErrChecksumMismatch  = errors.New("backup file checksum mismatch")
	ErrArchiveTooLarge   = errors.New("backup archive too large")
)

// MaxArchiveBytes limits uploaded restore archives (500 MiB).
const MaxArchiveBytes = 500 << 20
