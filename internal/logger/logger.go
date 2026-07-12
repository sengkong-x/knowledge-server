package logger

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, nil)
	return slog.New(handler)
}
