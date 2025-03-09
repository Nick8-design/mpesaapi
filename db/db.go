package db

import (
	"fmt"
	"log"
	"mpesa/models"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)


var Db *gorm.DB

func ConnectDb(){
	
	err:=godotenv.Load()
	if err!=nil{
		log.Fatal("Error loading env")
	}
	dsn:=os.Getenv("DB_url")
	if dsn==""{
		log.Fatal("Emty Db url")
	}
	Db,err=gorm.Open(postgres.Open(dsn),&gorm.Config{})
	if err!=nil{
		log.Fatal("Problem COnnecting to db")
	}

	fmt.Println("succefully connected to db")

	Db.AutoMigrate(&models.Payment{})
	fmt.Println("Table intialized")




}