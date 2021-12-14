package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func slowServer(w http.ResponseWriter, r *http.Request) {
	time.Sleep(10 * time.Second)
	w.Write([]byte("Hello world!"))
}

func call() error {
	client := &http.Client{Timeout: 1 * time.Second}
	req, err := http.NewRequest(http.MethodHead, "http://localhost:8080", nil)
	if err != nil {
		return err
	}
	// ctx, cancel := context.WithTimeout(req.Context(), 1*time.Second)
	// defer cancel()
	// req = req.WithContext(ctx)
	_, err = client.Do(req)
	return err
}

func main() {
	// run slow server
	go func() {
		http.HandleFunc("/", slowServer)

		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	time.Sleep(1 * time.Second) // wait for server to run

	// call server
	err := call()
	if errors.Is(err, context.DeadlineExceeded) {
		log.Println("ContextDeadlineExceeded: true")
		log.Println(err)
	}
	if os.IsTimeout(err) {
		log.Println("IsTimeoutError: true")
	}
	if err != nil {
		log.Fatal(err)
	}
}
