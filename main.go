package main

import (
	"bufio"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

type cmd struct {
	cmdName string
	cmdArgs []string
}

var commands = map[string]cmd{
	"slow-test": {"./slow-test.sh", []string{}},
}

var hostname = flag.String("hostname", "command", "hostname for the tailnet")

var localClient *tailscale.LocalClient

func main() {

	http.HandleFunc("/", home)
	http.HandleFunc("/run/", run)

	s := &tsnet.Server{
		Hostname: *hostname,
	}
	defer s.Close()

	ln, err := s.Listen("tcp", ":80")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	localClient, err = s.LocalClient()
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(http.Serve(ln, nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	who, err := localClient.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "<html><body><h1>ts-cmd</h1>\n")
	fmt.Fprintf(w, "<p>You are <b>%s</b> from <b>%s</b> (%s)</p>",
		html.EscapeString(who.UserProfile.LoginName),
		html.EscapeString(who.Node.ComputedName),
		r.RemoteAddr)

	fmt.Fprintf(w, "<ul>")
	for k, _ := range commands {
		fmt.Fprintf(w, "<li><a href=\"/run/%s\">%s</a></li>", k, k)
	}
	fmt.Fprintf(w, "</ul>")
}

func run(w http.ResponseWriter, r *http.Request) {

	// TODO only allow one person to be running a command at a time
	// TODO check for canceled requests and cancel the command

	cmdReq := strings.TrimPrefix(r.RequestURI, "/run/")
	if _, ok := commands[cmdReq]; !ok {
		http.Error(w, "no such cmd", 404)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "<html><body><h1>Running %s</h1><pre>", html.EscapeString(cmdReq))
	flusher.Flush()

	cmd := exec.Command(commands[cmdReq].cmdName, commands[cmdReq].cmdArgs...)

	stderr, _ := cmd.StdoutPipe()
	cmd.Start()

	scanner := bufio.NewScanner(stderr)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Fprintf(w, "%s\n", m)
		flusher.Flush()
	}
	cmd.Wait()
	fmt.Fprintf(w, "</pre>")
	flusher.Flush()
}
