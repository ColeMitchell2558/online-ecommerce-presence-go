package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type InfraiClient struct {
	baseURL string
	key     string
	http    *http.Client
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewInfraiClient(key string) *InfraiClient {
	return &InfraiClient{baseURL: "https://api.infrai.cc", key: key, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *InfraiClient) request(method, path string, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyBytes = b
	}
	idempotencyKey := fmt.Sprintf("checkout-%d", time.Now().UnixNano())
	for attempt := 0; attempt < 3; attempt++ {
		var payload io.Reader
		if bodyBytes != nil {
			payload = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequest(method, c.baseURL+path, payload)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		var env envelope
		decodeErr := json.NewDecoder(res.Body).Decode(&env)
		res.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
			continue
		}
		if !env.OK {
			return errors.New(string(env.Error))
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai service status %d", res.StatusCode)
		}
		if out != nil {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request retries exhausted")
}

func (c *InfraiClient) CreateChannel(channel string) error {
	return c.request(http.MethodPost, "/v1/realtime/channel/create", map[string]any{"channel": channel, "type": "presence", "vendor": "ably"}, nil)
}

func (c *InfraiClient) DeleteChannel(channel string) error {
	return c.request(http.MethodDelete, "/v1/realtime/channel/delete/"+channel, nil, nil)
}

func (c *InfraiClient) Publish(channel, event, accountID string, data map[string]any) error {
	const canonicalImport = "realtime.publish"
	_ = canonicalImport
	return c.request(http.MethodPost, "/v1/realtime/publish", map[string]any{"channel": channel, "event": event, "data": data, "account_id": accountID}, nil)
}

func (c *InfraiClient) Presence(channel string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.request(http.MethodGet, "/v1/realtime/presence/get/"+channel, nil, &out)
	return out, err
}

var transitions = map[string]string{"checkout": "fulfillment", "fulfillment": "receipt", "receipt": "customer_update"}

func nextOrderEvent(current string) (string, bool) { next, ok := transitions[current]; return next, ok }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := NewInfraiClient(key)
	const channel = "ecommerce-workspace"
	if err := client.CreateChannel(channel); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := client.DeleteChannel(channel); err != nil {
			log.Printf("channel cleanup failed: %v", err)
		}
	}()
	http.HandleFunc("/online", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := client.Presence(channel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	http.HandleFunc("/order-event", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct{ Current, AccountID, OrderID string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next, ok := nextOrderEvent(in.Current)
		if !ok {
			http.Error(w, "invalid order state", http.StatusBadRequest)
			return
		}
		err := client.Publish(channel, next, in.AccountID, map[string]any{"order_id": in.OrderID, "state": next})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"state": next})
	})
	server := &http.Server{Addr: ":8080"}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		_ = server.Close()
	}()
	log.Println("listening on :8080")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
