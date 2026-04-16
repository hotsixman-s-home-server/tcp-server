package main

import (
	"NDJFlow/module/server"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file.")
	}
}

func main() {
	checker, err := server.GetJSONKeyChecker()
	if err != nil {
		log.Fatal("Cannot read key.json")
		return
	}

	app, err := server.CreateServer(os.Getenv("PORT"), os.Getenv("UDS_PATH"), checker)
	if err != nil {
		log.Fatal("Cannot create server: ", err)
		return
	}

	app.Listen()
	log.Println("Listening...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	if app.TCPListener != nil {
		app.TCPListener.Close()
		log.Println("TCP Listener closed.")
	}
	if app.UDSListener != nil {
		app.UDSListener.Close()
		log.Println("UDS Listener closed (Socket file removed).")
	}
}
