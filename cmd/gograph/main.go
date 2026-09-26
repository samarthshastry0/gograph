package main

import (
	"log"
	"net/http"

	gograph "gograph"
)

func main() {
	if err := gograph.RegisterDemoTypes(); err != nil {
		log.Fatal(err)
	}

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", gograph.NewServer()))
}
