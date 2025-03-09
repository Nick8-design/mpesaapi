package main

import (
	"fmt"
	"log"
	"mpesa/db"
	"mpesa/handles"

	"github.com/gofiber/fiber/v2/middleware/logger"

	"github.com/gofiber/fiber/v2"
)

func init(){
	db.ConnectDb()
}

func main() {

	app := fiber.New()
	app.Use(logger.New())

	app.Post("/stkpush", handles.StkPushHandler)
	app.Post("/callback", handles.CallbackHandler)

	fmt.Println("Server running on port 8080")
	log.Fatal(app.Listen(":8080"))
}