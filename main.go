package main

import (
	"embed"
	"os"
	"os/signal"
	"syscall"

	"github.com/memsdm05/rpb/app"
)

//go:embed static/*
var content embed.FS

func main() {
	app.LoadConfig()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	app.Start(content)
	<-sigs
	app.Stop()
}
