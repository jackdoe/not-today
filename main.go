package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/sfreiberg/gotwilio"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const MIN_NOTIFICATION_INTERVAL_MS = int64(3600000)

type Failure struct {
	Error   string
	TookMs  int64
	StampMs int64
}

type HealthCheck struct {
	URL                string
	Failures           []*Failure
	notify             []string
	LastSuccessMs      int64
	LastFailureMs      int64
	LastNotificationMs int64
	sync.Mutex
}

func NewHealthCheck(url string, notify []string) *HealthCheck {
	check := &HealthCheck{
		URL:                url,
		Failures:           []*Failure{},
		LastNotificationMs: 0,
		notify:             notify,
	}

	go func() {
		tr := &http.Transport{
			DisableKeepAlives: true,
		}

		client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
		fixed := true
		go check.send(fmt.Sprintf("%s - started monitoring", check.URL))
		for {
			t0 := nowMs()
			resp, err := client.Get(check.URL)
			if err == nil {
				_, err = ioutil.ReadAll(resp.Body)
			}
			if err == nil && resp.StatusCode != 200 {
				err = errors.New(fmt.Sprintf("expected status code %d, but got %d", 200, resp.StatusCode))
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

				if t0-check.LastNotificationMs > MIN_NOTIFICATION_INTERVAL_MS && fixed {
					check.LastNotificationMs = t0
					go check.send(fmt.Sprintf("%s - failed, took: %d, error: %s", check.URL, failure.TookMs, err.Error()))
				}
				fixed = false
				check.LastFailureMs = t0
			} else {
				check.LastSuccessMs = t0
				fixed = true
			}
			time.Sleep(60 * time.Second)
		}
	}()
	return check
}

func (this *HealthCheck) send(message string) {
	for _, number := range this.notify {
		sendMessage(number, message)
	}
}

func nowMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

type TwilioAccount struct {
	Sid    string
	Token  string
	Number string
}

var TWILIO = TwilioAccount{}

func sendMessage(to, message string) {
	log.Printf("sending to %s : %s", to, message)
	gotwilio.NewTwilioClient(TWILIO.Sid, TWILIO.Token).SendSMS(TWILIO.Number, to, message, "", "")

}

func main() {
	var ptoken = flag.String("token", "", "twilio auth token from https://www.twilio.com/console")
	var psid = flag.String("sid", "", "twilio account sid from https://www.twilio.com/console")
	var pnumber = flag.String("number", "", "twilio phone number from https://www.twilio.com/console/phone-numbers/incoming")
	var pbind = flag.String("bind", ":8080", "ListenAndServe argument")
	var punparsedChecks = flag.String("checks", "", "comma separated list of health checks, for example: https://neko.science/@+123123123,https://google.com@+123123")
	flag.Parse()
	if *ptoken == "" || *psid == "" || *pnumber == "" {
		log.Fatalf("expected -token -sid and -number, use -help to see all options")
	}
	TWILIO.Sid = *psid
	TWILIO.Number = *pnumber
	TWILIO.Token = *ptoken

	checks := []*HealthCheck{}
	for _, site := range strings.Split(*punparsedChecks, ",") {
		splitted := strings.Split(site, "@")
		if len(splitted) == 1 {
			log.Fatalf("expected phone number to notify: %s", site)
		}
		url, notify := splitted[0], splitted[1:]
		checks = append(checks, NewHealthCheck(url, notify))
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(checks)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		}
	})

	log.Fatal(http.ListenAndServe(*pbind, nil))
}
