package main

import (
	"log"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/bootstrap"
)

func main() {
	app, err := bootstrap.New("configs")
	if err != nil {
		log.Fatalf("bootstrap application: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("run application: %v", err)
	}
}
