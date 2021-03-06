package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opendoor-labs/gong/phoenix"

	"github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/kidoman/embd"
	"github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/kidoman/embd/controller/pca9685"
	_ "github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/kidoman/embd/host/rpi" // This loads the RPi driver
	"github.com/opendoor-labs/gong/Godeps/_workspace/src/golang.org/x/net/context"
)

const (
	topicName   = "private:contracts"
	servoMin    = 350
	servoMax    = 650
	servoMinNew = 600
	servoMaxNew = 800
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

	// wake the servo controller so we can reset its channels
	if err := dev.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	resetAllChannels(dev)
	resetTimer := time.After(time.Second) // long enough for servos to reset

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

	select {
	case <-resetTimer: // servos have had enough time to reset
		if err := dev.Sleep(); err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		return
	}

	for {
		select {
		case evt := <-eventch:
			switch evt.Event {
			case "acquisition_contract", "resale_contract":
				handleRingEvent(dev, evt)
			case "system_test":
				handleSystemTest(dev, evt)
			default:
				log.Printf("unhandled message received: %#v", evt)
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
		setting := servoMin
		if i == bellChannels["resale_contract"] {
			if isNewHardware() {
				setting = chimeMaxNew
			} else {
				setting = chimeMax
			}
		} else if isNewHardware() {
			setting = servoMinNew
		}
		if err := d.SetPwm(i, 0, setting); err != nil {
			return err
		}
	}
	return nil
}

var bellChannels = map[string]int{
	"acquisition_contract": 5,
	"resale_contract":      6,
}

func handleRingEvent(dev *pca9685.PCA9685, evt *phoenix.Event) {
	payload := AddressPayload{}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		log.Printf("unmarshaling %s payload: %s", evt.Event, err)
		return
	}
	log.Printf("%s received: topic=%q ref=%q payload=%#v", evt.Event, evt.Topic, evt.Ref, payload)
	switch evt.Event {
	case "acquisition_contract":
		ringBell(dev, bellChannels[evt.Event])
	case "resale_contract":
		ringChime(dev, bellChannels[evt.Event])
	}
}

func handleSystemTest(dev *pca9685.PCA9685, evt *phoenix.Event) {
	payload := struct {
		DeviceID      string `json:"device_id"`
		SubsystemName string `json:"subsystem_name"`
	}{}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		log.Printf("unmarshaling %s payload: %s", evt.Event, err)
		return
	}

	if payload.DeviceID == "" || payload.DeviceID != os.Getenv("RESIN_DEVICE_UUID") {
		log.Printf("system test requested for device_id=%s, skipped with device_id=%d", payload.DeviceID, os.Getenv("RESIN_DEVICE_UUID"))
		return
	}
	switch payload.SubsystemName {
	case "bell":
		log.Printf("running system test with bell...")
		ringBell(dev, bellChannels["acquisition_contract"])
	case "chime":
		log.Printf("running system test with chime...")
		ringChime(dev, bellChannels["resale_contract"])
	default:
		log.Printf("running system test with both bell and chime...")
		ringBell(dev, bellChannels["acquisition_contract"])
		time.Sleep(time.Second)
		ringChime(dev, bellChannels["resale_contract"])
	}
}

func ringBell(d *pca9685.PCA9685, chanID int) {
	if isNewHardware() {
		ringBellNew(d, chanID)
		return
	}

	if err := d.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := d.SetPwm(chanID, 0, servoMax); err != nil {
		log.Fatal("setting to max: ", err)
	}
	time.Sleep(450 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, servoMin); err != nil {
		log.Fatal("setting to min: ", err)
	}
	time.Sleep(400 * time.Millisecond)
	if err := d.Sleep(); err != nil {
		log.Fatal("sleeping: ", err)
	}
}

func ringBellNew(d *pca9685.PCA9685, chanID int) {
	if err := d.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := d.SetPwm(chanID, 0, servoMaxNew); err != nil {
		log.Fatal("setting to max: ", err)
	}
	time.Sleep(450 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, servoMinNew); err != nil {
		log.Fatal("setting to min: ", err)
	}
	time.Sleep(400 * time.Millisecond)
	if err := d.Sleep(); err != nil {
		log.Fatal("sleeping: ", err)
	}
}

const (
	chimeMax    = 600
	chimeMin    = 330
	chimeMaxNew = 280
	chimeMinNew = 200
)

func ringChime(d *pca9685.PCA9685, chanID int) {
	if isNewHardware() {
		ringChimeNew(d, chanID)
		return
	}

	if err := d.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := d.SetPwm(chanID, 0, chimeMin); err != nil {
		log.Fatal("setting to min: ", err)
	}
	time.Sleep(120 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, (chimeMin+2*chimeMax)/3); err != nil {
		log.Fatal("setting to middle: ", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, chimeMin); err != nil {
		log.Fatal("setting to min 2: ", err)
	}
	time.Sleep(120 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, chimeMax); err != nil {
		log.Fatal("setting to max: ", err)
	}
	time.Sleep(400 * time.Millisecond)

	if err := d.Sleep(); err != nil {
		log.Fatal("sleeping: ", err)
	}
}

func ringChimeNew(d *pca9685.PCA9685, chanID int) {
	if err := d.Wake(); err != nil {
		log.Fatal("waking: ", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := d.SetPwm(chanID, 0, chimeMinNew); err != nil {
		log.Fatal("setting to min: ", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, chimeMaxNew); err != nil {
		log.Fatal("setting to middle: ", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, chimeMinNew); err != nil {
		log.Fatal("setting to min 2: ", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := d.SetPwm(chanID, 0, chimeMaxNew); err != nil {
		log.Fatal("setting to max: ", err)
	}
	time.Sleep(400 * time.Millisecond)

	if err := d.Sleep(); err != nil {
		log.Fatal("sleeping: ", err)
	}
}

func isNewHardware() bool {
	return os.Getenv("NEW_HARDWARE") != ""
}
