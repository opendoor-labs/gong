package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
)

const (
	topicName = "private:closes"
)

func main() {
	guardianToken := os.Getenv("GUARDIAN_TOKEN")

	dialer := &websocket.Dialer{}

	query := url.Values{}
	query.Set("vsn", "1.0.0")
	query.Set("guardian_token", guardianToken)
	u := url.URL{
		Scheme:   "wss",
		Host:     "opendoor-pusher.herokuapp.com",
		Path:     "/events/websocket",
		RawQuery: query.Encode(),
	}

	// per docs, this resp.Body doesn't need to be closed
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Printf("connected to %s\n", conn.RemoteAddr())

	// TODO: send ping messages every ~30s

	joinMsg := PhoenixEvent{
		Topic: topicName,
		Event: "phx_join",
		Ref:   "1",
	}
	joinPayload, err := json.Marshal(PhxJoinPayload{GuardianToken: guardianToken})
	if err != nil {
		log.Fatal(err)
	}
	joinMsg.Payload = joinPayload
	if err = conn.WriteJSON(&joinMsg); err != nil {
		log.Fatalf("joining %s: %s", topicName, err)
	}

	var msg PhoenixEvent
	for {
		if err = conn.ReadJSON(&msg); err != nil {
			log.Fatal(err)
		}
		switch msg.Event {
		case "phx_reply":
			payload := PhxReplyPayload{}
			if err = json.Unmarshal(msg.Payload, &payload); err != nil {
				fmt.Println("unmarshaling phx_reply payload:", err)
			}
			fmt.Printf("phx_reply received: topic=%q ref=%q payload=%#v\n", msg.Topic, msg.Ref, payload)
		case "acquisition_closed":
			payload := AcquisitionClosedPayload{}
			if err = json.Unmarshal(msg.Payload, &payload); err != nil {
				fmt.Println("unmarshaling acquisition_closed payload:", err)
			}
			fmt.Printf("acquisition_closed received: topic=%q ref=%q payload=%#v\n", msg.Topic, msg.Ref, payload)
		default:
			fmt.Printf("unhandled message received: %#v\n", msg)
		}
	}
}

type PhoenixEvent struct {
	Topic   string          `json:"topic"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Ref     string          `json:"ref"`
}

type AcquisitionClosedPayload struct {
	Address string
}

type PhxJoinPayload struct {
	GuardianToken string `json:"guardian_token"`
}

type PhxReplyPayload struct {
	Status   string                 `json:"status"`
	Response map[string]interface{} `json:"response"`
}
