package logger

import "os"

func stderr() *os.File {
	return os.Stderr
}
