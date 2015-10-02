package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/opendoor-labs/gong/phoenix"
	"github.com/opendoor-labs/gong/pwm"
	"golang.org/x/net/context"
)

const (
	topicName = "private:closes"
)

func main() {
	guardianToken := os.Getenv("GUARDIAN_TOKEN")
	if guardianToken == "" {
		log.Fatal("GUARDIAN_TOKEN is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
	go handleSignals(sigch, ctx, cancel)

	if err := pwm.Init(); err != nil {
		log.Fatal("PWM init: ", err)
	}
	defer pwm.Close()

	device, err := pwm.New(1, 0x40)
	if err != nil {
		log.Fatal("PWM new: ", err)
	}
	defer device.Close()

	query := url.Values{}
	query.Set("vsn", "1.0.0")
	query.Set("guardian_token", guardianToken)
	u := url.URL{
		Scheme:   "wss",
		Host:     "opendoor-pusher.herokuapp.com",
		Path:     "/events/websocket",
		RawQuery: query.Encode(),
	}

	joinPayload, err := json.Marshal(GuardianPayload{GuardianToken: guardianToken})
	if err != nil {
		log.Fatal(err)
	}

	client := phoenix.InitClient(u.String(), []string{topicName}, joinPayload)
	eventch := client.Start()
	defer client.Close()

	for {
		select {
		case evt := <-eventch:
			switch evt.Event {
			case "acquisition_closed", "resale_closed":
				payload := AddressPayload{}
				if err := json.Unmarshal(evt.Payload, &payload); err != nil {
					fmt.Printf("unmarshaling %s payload: %s\n", evt.Event, err)
				}
				fmt.Printf("%s received: topic=%q ref=%q payload=%#v\n", evt.Event, evt.Topic, evt.Ref, payload)
			default:
				fmt.Printf("unhandled message received: %#v\n", evt)
			}
		case <-ctx.Done():
			return
		}
	}
}

func handleSignals(sigch <-chan os.Signal, ctx context.Context, cancel context.CancelFunc) {
	select {
	case <-ctx.Done():
	case sig := <-sigch:
		switch sig {
		case os.Interrupt:
			log.Println("SIGINT")
		case syscall.SIGTERM:
			log.Println("SIGTERM")
		}
		cancel()
	}
}

type AddressPayload struct {
	Address string
}

type GuardianPayload struct {
	GuardianToken string `json:"guardian_token"`
}
