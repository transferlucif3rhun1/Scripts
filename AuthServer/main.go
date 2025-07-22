package main

import (
	"log"
	"net/http"
	"strconv"
	routes "AuthServer/routes"

	"github.com/gorilla/mux"
)


var PORT = 3000

func main() {

	r := mux.NewRouter()
	r.Use(Logger)

	r.HandleFunc("/", routes.DefaultHandler).Methods("GET")

	r.HandleFunc("/generate-api-key", routes.CreateKey).Methods("POST")

	r.HandleFunc("/update-api-key", routes.UpdateKey).Methods("POST")

	r.HandleFunc("/delete-api-key", routes.DeleteKey).Methods("POST")

	r.HandleFunc("/info-api-key", routes.ReadKey).Methods("POST")

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), r))
}