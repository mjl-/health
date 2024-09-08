package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mjl-/httpinfo"
	"github.com/mjl-/sconf"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type endpoint struct {
	URL  string
	Name string
}

var config struct {
	Endpoints []endpoint
}

var (
	vcsCommitHash = ""
	vcsTag        = ""
	vcsBranch     = ""
)

func check(err error, action string) {
	if err != nil {
		log.Fatalf("%s: %s", action, err)
	}
}

func main() {
	log.SetFlags(0)

	usage := func() {
		log.Fatal("usage: health [flags] { config | testconfig health.conf | serve health.conf | version }")
	}

	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "config":
		if len(args) != 0 {
			usage()
		}
		err := sconf.Describe(os.Stdout, &config)
		check(err, "describing config")
	case "testconfig":
		if len(args) != 1 {
			usage()
		}
		parseConfig(args[0])
		log.Print("config OK")
	case "serve":
		serve(args)
	case "version":
		if len(args) != 0 {
			usage()
		}
		fmt.Println(version)
	default:
		usage()
	}
}

func parseConfig(filename string) {
	err := sconf.ParseFile(filename, &config)
	check(err, "parsing config file")
	if len(config.Endpoints) == 0 {
		log.Fatal("need one or more endpoints")
	}
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	address := fs.String("address", "localhost:8000", "address to serve /health on")
	monitorAddress := fs.String("monitor-address", "localhost:8001", "address to serve monitoring endpoints on")
	fs.Usage = func() {
		log.Print("usage: health serve [flags] health.conf")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	args = fs.Args()
	if len(args) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	parseConfig(args[0])

	http.Handle("/metrics", promhttp.Handler())

	// Since we set the version variables with ldflags -X, we cannot read them in the vars section.
	// So we combine them into a CodeVersion during init, and add the handler while we're at it.
	info := httpinfo.CodeVersion{
		CommitHash: vcsCommitHash,
		Tag:        vcsTag,
		Branch:     vcsBranch,
		Full:       version,
	}
	http.Handle("/info", httpinfo.NewHandler(info, nil))

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", health)

	log.Printf("health version %s, listening on %s and monitor %s", version, *address, *monitorAddress)
	go func() {
		log.Fatal(http.ListenAndServe(*monitorAddress, nil))
	}()
	log.Fatal(http.ListenAndServe(*address, healthMux))
}

type result struct {
	endpoint
	OK bool
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	statusc := make(chan result, len(config.Endpoints))
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	for _, ep := range config.Endpoints {
		go checkEndpoint(ctx, ep, statusc)
	}

	var failed []string
	for range config.Endpoints {
		r := <-statusc
		if !r.OK {
			failed = append(failed, r.Name)
		}
	}
	if len(failed) > 0 {
		http.Error(w, "500 internal server error - unhealthy: "+strings.Join(failed, ", "), http.StatusInternalServerError)
	} else {
		fmt.Fprintln(w, "ok")
	}
}

func checkEndpoint(ctx context.Context, ep endpoint, statusc chan result) {
	defer func() {
		e := recover()
		statusc <- result{endpoint: ep, OK: e == nil}
		if e != nil {
			log.Printf("checking endpoint %s %s: %s", ep.Name, ep.URL, e)
		}
	}()

	type xerror struct{ error }

	xcheck := func(err error, action string) {
		if err != nil {
			panic(xerror{fmt.Errorf("%s: %s", action, err)})
		}
	}

	req, err := http.NewRequest("GET", ep.URL, nil)
	req = req.WithContext(ctx)
	xcheck(err, "http get")
	resp, err := http.DefaultClient.Do(req)
	xcheck(err, "http get")
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		xcheck(fmt.Errorf("status %d %s", resp.StatusCode, resp.Status), "http response")
	}
}
