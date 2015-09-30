package phoenix

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	u      string
	dialer *websocket.Dialer

	topics           []string
	topicJoinPayload []byte

	inboundc chan *Event

	mu     sync.Mutex
	closed bool
	donec  chan struct{}
	waitc  chan struct{}
}

func InitClient(url string, topics []string, topicJoinPayload []byte) *Client {
	return &Client{
		u:      url,
		dialer: &websocket.Dialer{},

		topics:           topics,
		topicJoinPayload: topicJoinPayload,
	}
}

func (c *Client) Start() (inboundEventCh <-chan *Event) {
	c.donec = make(chan struct{})
	c.waitc = make(chan struct{})
	c.inboundc = make(chan *Event)

	go c.connLoop()
	return c.inboundc
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.donec != nil && !c.closed {
		close(c.donec)
		<-c.waitc
		c.closed = true
	}
}

func (c *Client) connLoop() {
	for {
		err := c.connOnce(c.u)
		if err != nil {
			log.Printf("conn error: %s", err)
		}
		select {
		case <-c.donec:
			close(c.inboundc)
			close(c.waitc)
			return
			// TODO: backoff
		default:
			log.Println("disconnected, reconnecting")
		}
	}
}

func (c *Client) connOnce(url string) error {
	// per docs, this resp.Body doesn't need to be closed
	conn, _, err := c.dialer.Dial(c.u, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	fmt.Printf("connected to %s\n", conn.RemoteAddr())

	for _, topic := range c.topics {
		joinMsg := Event{
			Topic:   topic,
			Event:   "phx_join",
			Payload: c.topicJoinPayload,
			Ref:     "1",
		}
		if err = conn.WriteJSON(&joinMsg); err != nil {
			return err
		}
	}

	recvc := make(chan eventOrError, 1)
	go c.receiveMsg(conn, recvc)

	// TODO: send ping messages every ~30s

	for {
		select {
		case <-c.donec:
			conn.Close()
			return nil
		case eventOrErr := <-recvc:
			if eventOrErr.err != nil {
				return err
			}

			c.handleEvent(eventOrErr.event)
			go c.receiveMsg(conn, recvc)
		}
		// TODO: send ping messages every ~30s

	}
}

func (c *Client) handleEvent(evt *Event) {
	switch evt.Event {
	case "phx_reply":
		payload := PhxReplyPayload{}
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			fmt.Println("unmarshaling phx_reply payload:", err)
		}
		fmt.Printf("phx_reply received: topic=%q ref=%q payload=%#v\n", evt.Topic, evt.Ref, payload)
	default:
		select {
		case c.inboundc <- evt:
		default:
			fmt.Printf("no receiver ready, dropping message: %#v\n", evt)
		}
	}
}

func (c *Client) receiveMsg(conn *websocket.Conn, recvc chan<- eventOrError) {
	event := &Event{}
	if err := conn.ReadJSON(event); err != nil {
		recvc <- eventOrError{err: err}
		close(recvc)
		return
	}
	recvc <- eventOrError{event: event}
}

type eventOrError struct {
	event *Event
	err   error
}

type Event struct {
	Topic   string          `json:"topic"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Ref     string          `json:"ref"`
}

type PhxReplyPayload struct {
	Status   string                 `json:"status"`
	Response map[string]interface{} `json:"response"`
}

type TopicJoinFunc func(topic string) (payload string)
