package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

type Failure struct {
	Error   string
	TookMs  int64
	StampMs int64
}

func (f *Failure) JSON() string {
	b, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}
	return string(b)
}

type HealthCheck struct {
	URL                string
	Failures           []*Failure
	LastSuccessMs      int64
	LastFailureMs      int64
	LastNotificationMs int64
	Alive              bool
	sync.Mutex
}

func (h *HealthCheck) JSON() string {
	b, err := json.Marshal(h)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func run(command string, args ...string) {
	log.Printf("running: %v %v", command, args)

	cmd := exec.Command(command, args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}

	log.Printf("output: " + string(b))
}

func NewHealthCheck(url string, interval int, cmd string) *HealthCheck {
	check := &HealthCheck{
		URL:                url,
		Failures:           []*Failure{},
		LastNotificationMs: 0,
		Alive:              true,
	}

	go func() {
		tr := &http.Transport{
			DisableKeepAlives: true,
		}

		client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
		log.Printf("checking %v", check.URL)
		for {
			t0 := nowMs()
			resp, err := client.Get(check.URL)
			if err == nil {
				_, err = ioutil.ReadAll(resp.Body)
			}
			if err == nil && resp.StatusCode != 200 {
				err = fmt.Errorf("%s expected status code %d, but got %d", check.URL, 200, resp.StatusCode)
			}

			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if err != nil {
				failure := &Failure{
					Error:   err.Error(),
					TookMs:  nowMs() - t0,
					StampMs: t0,
				}
				check.Lock()
				check.Failures = append(check.Failures, failure)
				if len(check.Failures) > 100 {
					check.Failures = check.Failures[1:]
				}
				check.Unlock()

				check.LastNotificationMs = t0

				if check.Alive {
					go run(cmd, "false", err.Error())
				}

				check.Alive = false
				check.LastFailureMs = t0
			} else {
				check.LastSuccessMs = t0
				if !check.Alive {
					go run(cmd, "true", "")
				}
				check.Alive = true

			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
	return check
}

func nowMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func main() {
	var pexec = flag.String("cmd", "", "exec this command when healthcheck fails")
	var pbind = flag.String("bind", ":8080", "ListenAndServe argument")
	var pinterval = flag.Int("interval", 60, "run checks every N seconds")
	flag.Parse()
	if len(flag.Args()) == 0 {
		log.Fatal("pass urls after the arguments, e.g.: go run main.go -exec /tmp/bad.sh http://google.com http://yahoo.com")
	}

	if *pexec == "" {
		log.Fatalf("expected -exec command that will be executed when healthcheck fails")
	}

	checks := []*HealthCheck{}
	for _, site := range flag.Args() {
		checks = append(checks, NewHealthCheck(site, *pinterval, *pexec))
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(checks)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(b)
		}
	})

	log.Fatal(http.ListenAndServe(*pbind, nil))
}
