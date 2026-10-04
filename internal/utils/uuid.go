package utils

import (
	"log/slog"

	"github.com/google/uuid"
)

func GenUUIDv7() uuid.UUID {
	v7, err := uuid.NewV7()
	if err != nil {
		slog.Error("Generating uuid failed: ", slog.Any("error", err))
	}
	return v7
}
