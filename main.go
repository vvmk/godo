// Godo is a command line todo list client/server rolled together.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex
var count int

// A Todo is a thing I need to do...go figure.
type Todo struct {
	Body      string    `json:"body"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

// temporary in memory todo repo
var lists = make(map[string][]Todo)

var listName = flag.String("l", "Inbox", "name of the list to which this item will be added")

func main() {
	// TODO: zero command line args crashes
	flag.Parse()

	todo := strings.Join(flag.Args(), " ")

	if os.Args[1] == "ls" {
		getTodos()
		return
	}

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

	if os.Args[1] == "server" {
		startServer()
	}

	addTodo(*listName, todo)
}

// addTodo adds a todo item to the in-memory store under the given list
// name.
func addTodo(listName string, todo string) {
	url := "http://localhost:8001/create"

	body := struct {
		List     string `json:"list"`
		TodoBody string `json:"todo"`
	}{
		List:     listName,
		TodoBody: todo,
	}

	data, err := json.Marshal(body)
	if err != nil {
		log.Fatalf("JSON marshaling failed: %s", err)
	}

	fmt.Printf("body: %v\n", body)
	fmt.Printf("data: %s\n", data)

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "addTodo: POST to %s failed: %v\n", url, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("\nStatus: %v\n", resp.Status)
}

func testConnection() {
	// ping server
	fmt.Println("testing connection...not really")
}

// getTodos requests a dump of all todos saved to the server
func getTodos() {
	url := "http://localhost:8001/todos"

	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "getTodos: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "getTodos: reading %s: %v\n", url, err)
		os.Exit(1)
	}

	fmt.Printf("\nStatus: %v\n", resp.Status)
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
		defer resp.Body.Close()

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
	defer resp.Body.Close()

	nbytes, err := io.Copy(ioutil.Discard, resp.Body)
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
	http.HandleFunc("/count", counter)
	http.HandleFunc("/request", request)

	http.HandleFunc("/create", createTodo)
	http.HandleFunc("/todos", listTodos)

	log.Fatal(http.ListenAndServe("localhost:8001", nil))
}

// handler just echos back the url path
func handler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	count++
	mu.Unlock()

	fmt.Fprintf(w, "URL.Path = %q\n", r.URL.Path)
}

// counter echos the number of incoming requests
func counter(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	fmt.Fprintf(w, "Count %d\n", count)
	mu.Unlock()
}

// request echos the requests headers and form data for debugging calls
func request(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s %s %s\n", r.Method, r.URL, r.Proto)
	for k, v := range r.Header {
		fmt.Fprintf(w, "Header[%q] = %q\n", k, v)
	}

	fmt.Fprintf(w, "Host = %q\n", r.Host)
	fmt.Fprintf(w, "RemoteAddr = %q\n", r.RemoteAddr)

	if err := r.ParseForm(); err != nil {
		log.Print(err)
	}

	for k, v := range r.Form {
		fmt.Fprintf(w, "Form[%q] = %q\n", k, v)
	}
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var b struct {
		List string
		Todo string
	}

	err := decoder.Decode(&b)
	if err != nil {
		log.Panic(err)
	}

	t := Todo{
		Body:      b.Todo,
		Completed: false,
		CreatedAt: time.Now(),
	}

	key := b.List

	lists[key] = append(lists[key], t)

	thisTodo := lists[key][len(lists[key])-1]
	log.Printf("added todo: '%s' to list: '%s' at %v", thisTodo.Body, key, thisTodo.CreatedAt)

	tData, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(tData)
}

func listTodos(w http.ResponseWriter, r *http.Request) {
	rString, err := json.Marshal(lists)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rString)
}
