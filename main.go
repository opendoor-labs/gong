package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opendoor-labs/gong/phoenix"
	"golang.org/x/net/context"

	"github.com/kidoman/embd"
	"github.com/kidoman/embd/controller/pca9685"
	_ "github.com/kidoman/embd/host/rpi" // This loads the RPi driver
)

const (
	topicName = "private:contracts"
	servoMin  = 150
	servoMax  = 600
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

	if err := embd.InitI2C(); err != nil {
		log.Fatal("I2C init: ", err)
	}
	defer embd.CloseI2C()

	bus := embd.NewI2CBus(1)

	dev := pca9685.New(bus, 0x40)
	dev.Freq = 100
	defer dev.Close()

	resetAllChannels(dev)

	for i := 0; i < 3; i++ {
		ringBell(dev, 0)
		ringBell(dev, 2)
	}

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
			case "acquisition_contract", "resale_contract":
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

func resetAllChannels(d *pca9685.PCA9685) error {
	for i := 0; i < 16; i++ {
		if err := d.SetPwm(i, 0, servoMin); err != nil {
			return err
		}
	}
	return nil
}

func ringBell(d *pca9685.PCA9685, chanID int) {
	if err := d.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	if err := d.SetPwm(chanID, 0, servoMax); err != nil {
		log.Fatal("setting to max: ", err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := d.SetPwm(chanID, 0, servoMin); err != nil {
		log.Fatal("setting to min: ", err)
	}
	time.Sleep(time.Second)
	if err := d.Sleep(); err != nil {
		log.Fatal("waking: ", err)
	}
}
