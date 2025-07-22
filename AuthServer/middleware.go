package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/fatih/color"
)



// Middleware Logging Function
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Print the log to the console.
		fmt.Printf("[%s] | [%s] | [%s] | [%s]\n",
		    color.RedString("AuthServer"),
			color.BlueString("%s", time.Now().Format(time.RFC3339)), // Current Time
			color.GreenString("%s", r.Method),                       // Request Method
			color.YellowString(r.URL.Path),                          // Request Path 
		)

		next.ServeHTTP(w, r)
	})
}