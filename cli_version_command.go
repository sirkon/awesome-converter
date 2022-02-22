package main

import (
	"runtime/debug"

	"awesome-converter/internal/app"
	"github.com/sirkon/message"
)

// VersionCommand команда показа версии приложения
type VersionCommand struct{}

// Run запуск команды
func (*VersionCommand) Run(rctx *RunContext) error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		message.Warning(
			"WARNING: you are using a version compiled with modules disabled, this is not the way it supposed to be",
		)
	} else {
		message.Info(app.Name, "version", info.Main.Version)
	}

	return nil
}
