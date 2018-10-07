// Godo is a command line todo list client/server rolled together.
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	// fetch a single url
	if os.Args[1] == "fetch" {
		fetch(os.Args[2:])
		return
	}

	// fetch all urls in the arg list
	if os.Args[1] == "fetchall" {
		fetchall(os.Args[2:])
		return
	}

	// get text passed to godo
	input := strings.Join(os.Args[1:], " ")

	// spit it back out
	fmt.Println(input)
}

func testConnection() {
	// ping server
	fmt.Println("testing connection...not really")
}

// fetch makes a GET request to the supplied url strings (args) and
// prints the resulting response body, or an error.
func fetch(args []string) {
	for _, url := range args {
		url = ensureProtocol(url)

		resp, err := http.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
			os.Exit(1)
		}

		_, err = io.Copy(os.Stdout, resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch: reading %s: %v\n", url, err)
			os.Exit(1)
		}

		fmt.Printf("\nStatus: %v\n", resp.Status)
	}
}

// ensureProtocol adds the http:// protocol to a supplied url if it is
// not present
func ensureProtocol(url string) string {
	protocol := "http://"
	if !strings.HasPrefix(url, protocol) {
		return protocol + url
	}

	return url
}

// fetchall asyncronously fetches data from all urls supplied in
// args
func fetchall(args []string) {
	start := time.Now()
	ch := make(chan string)
	for _, url := range args {
		go fetchC(url, ch) // start a goroutine
	}
	for range args {
		fmt.Println(<-ch) // receive from channel ch
	}
	fmt.Printf("%.2fs elapsed\n", time.Since(start).Seconds())
}

// fetchC makes a GET request to a supplied url and writes a summary
// or an error to channel ch
func fetchC(url string, ch chan<- string) {
	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		ch <- fmt.Sprint(err) // send to channel ch
		return
	}

	nbytes, err := io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close() // don't leak resources
	if err != nil {
		ch <- fmt.Sprintf("while reading %s: %v", url, err)
		return
	}

	secs := time.Since(start).Seconds()
	ch <- fmt.Sprintf("%.2fs	%7d	%s", secs, nbytes, url)
}

// startServer starts a server that will receive and store todo messages
// and respond to list queries from godo clients
func startServer() {
	http.HandleFunc("/", handler) // each request calls handler
	log.Fatal(http.ListenAndServe("localhost:8000", nil))
}

// handler just echos back the url path
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "URL.Path = %q\n", r.URL.Path)
}
