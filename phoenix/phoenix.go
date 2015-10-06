package phoenix

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/gorilla/websocket"
	"github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/jpillora/backoff"
)

type Client struct {
	u                     string
	dialer                *websocket.Dialer
	heartbeatInterval     time.Duration
	heartbeatTimeout      time.Duration
	heartbeatTimeoutTimer *time.Timer

	topics           []string
	topicJoinPayload []byte

	inboundc chan *Event

	mu     sync.Mutex
	closed bool
	donec  chan struct{}
	waitc  chan struct{}
	ref    int
}

func InitClient(url string, topics []string, topicJoinPayload []byte) *Client {
	return &Client{
		u: url,
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
		heartbeatInterval: 30 * time.Second,
		heartbeatTimeout:  time.Minute,

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
	b := backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	for {
		err := c.connOnce(c.u, b.Reset)
		if err != nil {
			log.Printf("conn error: %s", err)
		}
		log.Println("disconnected")
		select {
		case <-c.donec:
			close(c.inboundc)
			close(c.waitc)
			return
			// TODO: backoff
		case <-time.After(b.Duration()):
			log.Println("reconnecting")
		}
	}
}

func (c *Client) connOnce(url string, f func()) error {
	// per docs, this resp.Body doesn't need to be closed
	conn, _, err := c.dialer.Dial(c.u, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	fmt.Printf("connected to %s\n", conn.RemoteAddr())
	if f != nil {
		f()
	}

	c.heartbeatTimeoutTimer = time.NewTimer(c.heartbeatTimeout)
	defer c.heartbeatTimeoutTimer.Stop()

	hbTick := time.NewTicker(c.heartbeatInterval)
	defer hbTick.Stop()

	for _, topic := range c.topics {
		joinMsg := Event{
			Topic:   topic,
			Event:   "phx_join",
			Payload: c.topicJoinPayload,
			Ref:     c.makeRef(),
		}
		if err = conn.WriteJSON(&joinMsg); err != nil {
			return err
		}
	}

	recvc := make(chan eventOrError, 1)
	go c.receiveMsg(conn, recvc)

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
		case <-hbTick.C:
			if err = c.sendHeartbeat(conn); err != nil {
				return err
			}
		case <-c.heartbeatTimeoutTimer.C:
			return fmt.Errorf("timeout waiting for heartbeat")
		}
	}
}

func (c *Client) handleEvent(evt *Event) {
	if evt.Topic == "phoenix" && evt.Event == "heartbeat" {
		c.heartbeatTimeoutTimer.Reset(c.heartbeatTimeout)
		return
	}
	switch evt.Event {
	case "phx_reply":
		payload := PhxReplyPayload{}
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			log.Println("unmarshaling phx_reply payload:", err)
		}
		log.Printf("phx_reply received: topic=%q ref=%q payload=%#v\n", evt.Topic, evt.Ref, payload)
	default:
		select {
		case c.inboundc <- evt:
		default:
			log.Printf("no receiver ready, dropping message: %#v\n", evt)
		}
	}
}

// makeRef returns the next message ref, accounting for overflows
func (c *Client) makeRef() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	newRef := c.ref + 1
	if newRef < c.ref {
		newRef = 0
	}
	c.ref = newRef
	return strconv.Itoa(newRef)
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

func (c *Client) sendHeartbeat(conn *websocket.Conn) error {
	hbMsg := Event{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: []byte("{}"),
		Ref:     c.makeRef(),
	}
	return conn.WriteJSON(&hbMsg)
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
